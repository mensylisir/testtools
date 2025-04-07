# TestTools Operator

TestTools 是一个基于 Kubebuilder 的 Kubernetes Operator，用于在 Kubernetes 集群中执行常见的网络测试工具。它使测试变得简单且可声明式，支持测试结果的收集和报告。

## 功能特性

- **声明式 API**：使用 Kubernetes 自定义资源声明测试
- **多种测试工具**：目前支持 `dig` DNS 测试
- **测试报告**：自动收集和汇总测试结果
- **监控集成**：可与 Prometheus 等监控系统集成
- **可扩展性**：基于控制器模式，易于添加新的测试工具

## 支持的测试工具

1. **Dig**：执行 DNS 解析测试

## 快速开始

### 先决条件

- Kubernetes 集群（v1.16+）
- kubectl（配置好集群访问）

### 安装 TestTools Operator

```bash
# 部署 CRD
kubectl apply -f config/crd/bases/

# 部署控制器
kubectl apply -f deploy/controller.yaml

# 检查部署状态
kubectl get pods -n testtools-system
```

### DNS 测试示例

#### 基本 DNS 查询

```bash
# 创建基本 DNS 查询测试
kubectl apply -f examples/dig/basic-dig.yaml

# 查看结果
kubectl describe dig dig-basic
```

#### 使用 Google DNS 服务器

```bash
# 创建使用特定 DNS 服务器的测试
kubectl apply -f examples/dig/dig-google-dns.yaml

# 查看结果
kubectl describe dig dig-google-dns
```

#### 查看所有 DNS 测试

```bash
kubectl get digs
```

### 创建测试报告

```bash
# 创建测试报告收集指定测试结果
kubectl apply -f examples/testreport/dns-testreport.yaml

# 查看测试报告
kubectl describe testreport dns-test-report
```

## 自定义资源

### Dig

Dig 资源用于执行 DNS 查询测试，可以配置多种查询参数。

请参阅 [Dig 资源文档](examples/dig/README.md) 获取详细说明。

### TestReport

TestReport 资源用于收集和汇总测试结果，生成测试报告。

请参阅 [TestReport 资源文档](examples/testreport/testreport-description.md) 获取详细说明。

## 路线图

- [ ] 添加更多网络测试工具（ping、traceroute 等）
- [ ] 支持 Prometheus 指标导出
- [ ] Web UI 界面查看测试结果
- [ ] 图表和可视化报告

## 贡献

欢迎贡献代码、报告问题或提出功能建议！

## 许可证

Apache 2.0 