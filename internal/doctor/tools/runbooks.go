// 本文件：从部署者提供的 Markdown 手册建立一个只读关键词索引。
//
// 手册是排查方法，不是当前故障的事实。检索只做文字匹配，不调用模型，不读取
// 场景答案。文件在启动时读一次，正式评测期间不会因为宿主机修改而悄悄换内容。
//
// 主要内容：LoadRunbooks 读取有界文件并计算内容哈希；Search 用 BM25 排序；
// WithRunbooks 把检索接到原有工具注册表；searchWords 处理中英文关键词。
// 阅读建议：先看 Search 的词频、稀有程度和长度修正，再看统一入口如何限制返回大小。

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Runbooks 构造后不修改；所有诊断可以共用同一份文档和词频统计。
type Runbooks struct {
	chunks    []runbookChunk // 按段落切分，结果不会把整本手册塞进上下文。
	frequency map[string]int // 某个词在多少段里出现，同段重复只算一次。
	average   float64        // 平均段落词数，避免长段仅凭篇幅排在前面。
	Hash      string         // 文件名和原始正文的 SHA-256，评测冻结时保存。
}

// runbookChunk 只保存手册正文，程序不会替它生成故障结论。
type runbookChunk struct {
	file, title, text string         // 返回片段的出处。
	terms             map[string]int // 本段词频。
	length            int            // 本段全部词数，包括重复出现的词。
	part              int            // 同一文件内的稳定片段编号。
}

// RunbookHit 的分数只是关键词相关程度，不能解释成诊断置信度。
type RunbookHit struct {
	File  string  `json:"file"`  // 固定目录内的文件名，不接受调用者提供路径。
	Title string  `json:"title"` // 便于人回到手册阅读上下文。
	Part  int     `json:"part"`  // 从一开始的片段编号。
	Score float64 `json:"score"` // BM25 得分，仅用于排序。
	Text  string  `json:"text"`  // 原始段落，不经模型改写。
}

// LoadRunbooks 拒绝子目录、符号链接和过大的文件，不能借手册目录间接读入仓库答案。
func LoadRunbooks(files fs.FS) (*Runbooks, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("读取排障手册目录：%w", err)
	}
	if len(entries) > 64 {
		return nil, fmt.Errorf("排障手册最多 64 个文件")
	}
	index := &Runbooks{frequency: map[string]int{}}
	digest := sha256.New()
	total := 0
	for _, entry := range entries {
		if entry.Name() == "README.md" {
			continue
		} // 导航说明不是供模型检索的排查正文。
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil, fmt.Errorf("手册目录只能包含普通 Markdown 文件")
		}
		file, err := files.Open(entry.Name())
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, 32769))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(raw) > 32768 || !utf8.Valid(raw) {
			return nil, fmt.Errorf("手册必须是最多 32KiB 的 UTF-8 文本")
		}
		fmt.Fprintf(digest, "%s\x00%d\x00", entry.Name(), len(raw))
		_, _ = digest.Write(raw)
		body := strings.ReplaceAll(string(raw), "\r\n", "\n")
		title := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(body, "\n", 2)[0], "#"))
		part := 0
		for _, paragraph := range strings.Split(body, "\n\n") {
			paragraph = strings.TrimSpace(paragraph)
			if paragraph == "" || strings.HasPrefix(paragraph, "# ") {
				continue
			}
			// 超长段分成多个完整 UTF-8 片段，后半段也进入索引，而不是被丢弃。
			for paragraph != "" {
				text := prefix(paragraph, 1500)
				paragraph = paragraph[len(text):]
				terms := searchWords(title + " " + text)
				part++
				counts := map[string]int{}
				for _, term := range terms {
					counts[term]++
				}
				for term := range counts {
					index.frequency[term]++
				}
				index.chunks = append(index.chunks, runbookChunk{entry.Name(), title, text, counts, len(terms), part})
				total += len(terms)
			}
		}
	}
	if len(index.chunks) == 0 || total == 0 {
		return nil, fmt.Errorf("排障手册没有可检索正文")
	}
	index.average = float64(total) / float64(len(index.chunks))
	index.Hash = hex.EncodeToString(digest.Sum(nil))
	return index, nil
}

// Search 使用固定 BM25 参数，不从当前实验的正确答案学习权重。
func (index *Runbooks) Search(ctx context.Context, query string, limit int) ([]RunbookHit, error) {
	if limit < 1 || limit > 3 || utf8.RuneCountInString(query) > 256 {
		return nil, fmt.Errorf("%w：检索限一至三段，关键词最多 256 字", ErrInvalidArgument)
	}
	terms := map[string]bool{}
	for _, word := range searchWords(query) {
		terms[word] = true
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("%w：请提供中文或英文关键词", ErrInvalidArgument)
	}
	ordered := make([]string, 0, len(terms))
	for term := range terms {
		ordered = append(ordered, term)
	}
	sort.Strings(ordered) // 固定浮点加法次序，重复检索不受 map 遍历顺序影响。
	hits := []RunbookHit{}
	for _, chunk := range index.chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		score := 0.0
		for _, term := range ordered {
			frequency := float64(chunk.terms[term])
			if frequency == 0 {
				continue
			}
			// 正的 IDF 形式与 Lucene BM25 一致。常见词的权重较低，但不会产生负分。
			df, count := float64(index.frequency[term]), float64(len(index.chunks))
			idf := math.Log1p((count - df + 0.5) / (df + 0.5))
			const k1, b = 1.2, 0.75 // 固定饱和程度和长度修正强度，正式评测不调参。
			score += idf * frequency * (k1 + 1) / (frequency + k1*(1-b+b*float64(chunk.length)/index.average))
		}
		if score > 0 {
			hits = append(hits, RunbookHit{chunk.file, chunk.title, chunk.part, score, chunk.text})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Part < hits[j].Part // 同分时固定顺序，不能因 map 顺序改变返回片段。
	})
	return hits[:min(limit, len(hits))], nil
}

// WithRunbooks 返回独立菜单；基础目录和 MCP 菜单不会因此得到 E2 专用工具。
func (registry *Registry) WithRunbooks(index *Runbooks) *Registry {
	result := registry.ForDiagnosis(time.Time{}, nil)
	result.add("runbook_search", "检索通用排查方法。片段是参考资料，不是当前系统的故障事实。", map[string]Property{"query": textProperty("中文或英文关键词", 1, 256), "limit": integerProperty("最多返回的片段数", 1, 3, 3)}, []string{"query"}, func(ctx context.Context, args arguments) (output, error) {
		hits, err := index.Search(ctx, args.text("query"), args.integer("limit"))
		return output{data: map[string]any{"runbooks_hash": index.Hash, "hits": hits}, summary: fmt.Sprintf("匹配 %d 段通用方法，相关分数不是根因概率", len(hits))}, err
	})
	return result
}

// searchWords 对英文保留单词，对中文使用相邻双字；不依赖词典，也不会下载分词模型。
func searchWords(text string) []string {
	// 遇到文字种类切换或标点时再结束一段，避免把中英文拼成没有意义的一个词。
	words := []string{}
	var latin, han []rune
	flush := func() {
		if len(latin) > 0 {
			words = append(words, string(latin))
			latin = nil
		}
		if len(han) == 1 {
			words = append(words, string(han))
		}
		for i := 1; i < len(han); i++ {
			words = append(words, string(han[i-1:i+1]))
		}
		han = nil
	}
	for _, char := range strings.ToLower(text) {
		switch {
		case unicode.Is(unicode.Han, char):
			if len(latin) > 0 {
				flush()
			}
			han = append(han, char)
		case (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_':
			if len(han) > 0 {
				flush()
			}
			latin = append(latin, char)
		default:
			flush()
		}
	}
	flush()
	return words
}
