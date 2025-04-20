@echo off
REM Windows批处理文件用于构建Docker镜像

set REGISTRY=registry.dev.rdev.tech:18093
set VERSION=v1

echo 构建FIO镜像...
docker build -t %REGISTRY%/testtools-fio:%VERSION% -f Dockerfile.fio .
docker push %REGISTRY%/testtools-fio:%VERSION%

echo 构建DIG镜像...
docker build -t %REGISTRY%/testtools-dig:%VERSION% -f Dockerfile.dig .
docker push %REGISTRY%/testtools-dig:%VERSION%

echo 构建PING镜像...
docker build -t %REGISTRY%/testtools-ping:%VERSION% -f Dockerfile.ping .
docker push %REGISTRY%/testtools-ping:%VERSION%

echo 构建NC镜像...
docker build -t %REGISTRY%/testtools-nc:%VERSION% -f Dockerfile.nc .
docker push %REGISTRY%/testtools-nc:%VERSION%

echo 构建TCPPING镜像...
docker build -t %REGISTRY%/testtools-tcpping:%VERSION% -f Dockerfile.tcpping .
docker push %REGISTRY%/testtools-tcpping:%VERSION%

echo 构建SKOOP镜像...
docker build -t %REGISTRY%/testtools-skoop:%VERSION% -f Dockerfile.skoop .
docker push %REGISTRY%/testtools-skoop:%VERSION%

echo 构建IPERF镜像...
docker build -t %REGISTRY%/testtools-iperf:%VERSION% -f Dockerfile.iperf .
docker push %REGISTRY%/testtools-iperf:%VERSION%

echo 构建控制器镜像...
docker build -t %REGISTRY%/testtools-controller:v70 .
docker push %REGISTRY%/testtools-controller:v70

echo 所有镜像已构建并推送完成 