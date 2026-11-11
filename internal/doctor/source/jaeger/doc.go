// Package jaeger 把 Jaeger 的调用链整理为诊断需要的服务、步骤和父子关系。服务名来自 process 映射，直接父节点必须属于同一条调用链。
//
// 从 Search 或 Get 跟到 decode。只有部署者能指定固定路径前缀，编码路径、点路径和重定向都会被拒绝。
//
//   - client.go：校验固定部署前缀，查询调用链并提取允许用于诊断的属性。
package jaeger
