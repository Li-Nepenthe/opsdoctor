# tools：代码检查与恢复验收

这里的工具不参与业务，只在开发时和 CI 里运行，用来检查项目规范，并验证医生进程崩溃后能否恢复。

| 工具 | 检查什么 |
|---|---|
| [commentcheck](commentcheck/doc.go) | 说明有没有写全：每个包有 `doc.go`、每个文件开头有“本文件：”说明、每个函数和类型有注释。还能生成函数清单（`-list`）和代码地图（`-map`） |
| [archcheck](archcheck/doc.go) | 项目特有的三类约束：文件位置、标准答案读取、医生 Compose 挂载与内部端口 |
| [depguard 配置](../.golangci.yml) | 通过 golangci-lint 检查导入方向和文件角色，例如 platform 不依赖业务、领域流程不直接访问数据库 |

运行方法（在仓库根目录）：

```text
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
golangci-lint run ./...
golangci-lint run --build-tags=leakbug ./...
```

前两个工具通过时各输出一行“通过”；golangci-lint 输出 `0 issues.`。它按构建条件选择文件，
所以默认版本与 `leakbug` 故障版本都要检查。安装固定版本的命令见 [AGENTS.md](../AGENTS.md)。

## 从哪里开始读

commentcheck 和 archcheck 都要分析 Go 的语法树（`go/ast`），对初学者偏难，建议放在阶段 0 的最后读。读的时候先看 `main.go`，弄清工具的输入和输出，再看 `check.go` 里的规则，最后看测试：测试里故意写了违规的代码，用来证明规则真的能发现问题。

[recoverycheck](recoverycheck/doc.go) 是阶段 7 的进程验收包。它真的终止自己创建的 Mock 子进程，再从专用医生测试库接续工作。运行方式见根 README 的集成测试步骤。它放在医生源码之外，医生不能启动外部命令的规则仍然适用。
