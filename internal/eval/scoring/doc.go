// Package scoring 按固定规则从工具记录核对证据。先看 Evidence，再读 fullScan 和 missingIndex 如何依照真实列判断索引和扫描。每项判断都能回到原始步骤。
//
//   - evidence.go：精确比较组件，检查工具返回的证据并计算覆盖率。
package scoring
