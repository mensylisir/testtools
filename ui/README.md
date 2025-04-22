# TestTools UI - DNS查询工具界面

这个项目是TestTools的前端界面，用于与Kubernetes中的Dig、Ping、FIO控制器交互。

## 功能特性

- 查看所有DNS查询(Dig)资源列表
- 创建新的DNS查询任务
- 查看DNS查询详情和结果
- 删除DNS查询任务
- 实时进度条显示查询状态
- 查看与DNS查询相关的测试报告(TestReport)

## 技术栈

- Vue 3
- Vue Router
- Axios
- Vite

## 安装和运行

### 安装依赖

```bash
npm install
```

### 开发模式运行

```bash
npm run dev
```

### 构建生产版本

```bash
npm run build
```

### 预览生产构建

```bash
npm run preview
```

## 配置Kubernetes API访问

默认情况下，UI会通过`/api`路径代理到Kubernetes API服务器。您可以通过环境变量`API_URL`修改API服务器地址。

## 部署到Kubernetes集群

1. 构建UI镜像:

```bash
docker build -t testtools-ui:latest .
```

2. 将UI镜像推送到您的镜像仓库

3. 创建部署和服务:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testtools-ui
spec:
  replicas: 1
  selector:
    matchLabels:
      app: testtools-ui
  template:
    metadata:
      labels:
        app: testtools-ui
    spec:
      containers:
      - name: testtools-ui
        image: your-registry/testtools-ui:latest
        ports:
        - containerPort: 80
        env:
        - name: API_URL
          value: "http://kubernetes.default.svc"
---
apiVersion: v1
kind: Service
metadata:
  name: testtools-ui
spec:
  selector:
    app: testtools-ui
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
```

## 创建新的DNS查询

通过界面创建的DNS查询将作为Kubernetes自定义资源创建在集群中。

## 项目结构

- `src/api` - API请求处理
- `src/components` - 可复用组件
- `src/views` - 页面视图
- `src/router` - 路由配置
- `src/assets` - 静态资源 