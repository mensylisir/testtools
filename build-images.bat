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

echo 构建控制器镜像...
docker build -t %REGISTRY%/testtools-controller:v70 .
docker push %REGISTRY%/testtools-controller:v70

echo 所有镜像已构建并推送完成 