# Kubernetes Test Tools Controller

A Kubernetes operator for managing network and storage performance tests in Kubernetes environments. This controller automates the execution, scheduling, and monitoring of various tests including Ping, Dig, and FIO (Flexible I/O Tester).

![Architecture Diagram](docs/images/architecture.png)

## Table of Contents

- [Overview](#overview)
- [Functionality](#functionality)
- [Installation](#installation)
- [Usage](#usage)
  - [Ping Tests](#ping-tests)
  - [Dig Tests](#dig-tests)
  - [FIO Tests](#fio-tests)
- [Advanced Features](#advanced-features)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Overview

The Kubernetes Test Tools Controller extends Kubernetes with custom resources to manage and execute network and storage performance tests. It helps operators validate infrastructure performance, troubleshoot connectivity issues, and establish performance baselines.

## Functionality

This controller provides the following core capabilities:

1. **Network Tests**:
   - **Ping Tests**: Measure connectivity and latency to specified endpoints
   - **Dig Tests**: Validate DNS resolution and performance
   - **Nc Tests**
   - **Iperf Tests**
   - **Tcpping tests**

2. **Storage Tests**:
   - **FIO Tests**: Benchmark disk I/O performance with flexible configurations

3. **Test Report Management**:
   - Automated collection of test results
   - Historical data tracking
   - Status monitoring

4. **Scheduling**:
   - Cron-based scheduling of recurring tests
   - One-time test execution

## Installation

### Prerequisites

- Kubernetes cluster v1.19+
- kubectl configured to communicate with your cluster
- cert-manager v1.0.0+ (for webhook support)

### Installation Steps

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/kubernetes-test-tools.git
   cd kubernetes-test-tools
   ```

2. Install Custom Resource Definitions (CRDs):
   ```bash
   make install
   ```

3. Deploy the controller:
   ```bash
   make deploy
   ```

4. Verify the installation:
   ```bash
   kubectl get pods -n testtools-system
   ```

## Usage

### Ping Tests

Create a YAML file for a ping test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Ping
metadata:
  name: example-ping-test
spec:
  host: "8.8.8.8"
  count: 5
```

Apply the file:
```bash
kubectl apply -f ping-test.yaml
```

Check the results:
```bash
kubectl get ping example-ping-test -o yaml
```

### Dig Tests

Create a YAML file for a DNS dig test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Dig
metadata:
  name: example-dig-test
spec:
  host: "kubernetes.default.svc.cluster.local"
  type: "A"
```

Apply the file:
```bash
kubectl apply -f dig-test.yaml
```

Check the results:
```bash
kubectl get dig example-dig-test -o yaml
```

### nc Tests

Create a YAML file for a DNS dig test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Nc
metadata:
   name: nc-sample-simple
spec:
   host: "kubernetes.io"
   port: 443
   timeout: 5
   verbose: true
```

Apply the file:
```bash
kubectl apply -f nc-sample-simple.yaml
```

Check the results:
```bash
kubectl get dig nc-sample-simple -o yaml
```


### tcpping Tests

Create a YAML file for a DNS dig test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: TcpPing
metadata:
   name: tcpping-sample-simple
spec:
   host: "kubernetes.io"
   port: 443
   count: 5
   timeout: 3
   interval: 1
```

Apply the file:
```bash
kubectl apply -f tcpping-sample-simple.yaml
```

Check the results:
```bash
kubectl get dig tcpping-sample-simple -o yaml
```

### iperf Tests

Create a YAML file for a DNS dig test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Iperf
metadata:
   name: iperf-sample-simple
spec:
   port: 5201
   duration: 10
   protocol: "tcp"
   verbose: true
```

Apply the file:
```bash
kubectl apply -f iperf-sample-simple.yaml
```

Check the results:
```bash
kubectl get dig iperf-sample-simple -o yaml
```

### FIO Tests

Create a YAML file for an FIO storage test:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Fio
metadata:
  name: example-fio-test
spec:
  filePath: "/data/fio-test"
  readWrite: "randread"
  blockSize: "4k"
  ioDepth: 32
  size: "1g"
  ioEngine: "libaio"
```

Apply the file:
```bash
kubectl apply -f fio-test.yaml
```

Check the results:
```bash
kubectl get fio example-fio-test -o yaml
```



### TestReport自动创建

使用本控制器的一个重要优势是TestReport会自动创建。用户只需创建Fio、Ping或Dig资源，控制器会：

1. 自动为每个测试资源创建对应的TestReport资源
2. 自动收集测试结果并更新报告
3. 自动维护测试历史和统计数据

例如，创建一个Fio资源后：

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Fio
metadata:
  name: my-fio-test
spec:
  filePath: "/data/test-file"
  readWrite: "randread"
  # 其他配置...
```

控制器会自动创建名为"fio-my-fio-test-report"的TestReport资源。无需手动创建或关联TestReport资源，整个过程完全自动化。

### Test Report Retrieval

Get the complete test report:

```bash
kubectl get testreport <report-name> -o yaml
```

### Error Handling

Configure error handling and retries:

```yaml
apiVersion: testtools.xiaoming.com/v1
kind: Fio
metadata:
  name: fio-with-retries
spec:
  # ... other configurations
  retries: 3
  retryInterval: 60  # seconds
  timeoutSeconds: 600
```

## Development

### Setting Up Development Environment

1. Install dependencies:
   ```bash
   go mod download
   ```

2. Run the controller locally:
   ```bash
   make run
   ```

### Building

Build the controller:
```bash
make build
```

### Testing

Run tests:
```bash
make test
```

### Adding New Test Types

To add a new test type:

1. Define the CRD in `api/v1/`
2. Implement the controller in `controllers/`
3. Update the `main.go` file to include the new controller
4. Generate the CRD manifests with `make manifests`

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.

## Contact

For issues or suggestions, please open an issue in the GitHub repository.
