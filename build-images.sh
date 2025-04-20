#!/bin/bash
set -e

REGISTRY="192.168.31.34:18093"
VERSION="v3"

# 构建并推送FIO镜像
echo "构建FIO镜像..."
docker build -t ${REGISTRY}/testtools-fio:${VERSION} -f Dockerfile.fio .
docker push ${REGISTRY}/testtools-fio:${VERSION}

# 构建并推送DIG镜像
echo "构建DIG镜像..."
docker build -t ${REGISTRY}/testtools-dig:${VERSION} -f Dockerfile.dig .
docker push ${REGISTRY}/testtools-dig:${VERSION}

# 构建并推送PING镜像
echo "构建PING镜像..."
docker build -t ${REGISTRY}/testtools-ping:${VERSION} -f Dockerfile.ping .
docker push ${REGISTRY}/testtools-ping:${VERSION}

echo "构建NC镜像..."
docker build -t ${REGISTRY}/testtools-nc:${VERSION} -f Dockerfile.nc .
docker push ${REGISTRY}/testtools-nc:${VERSION}

echo "构建TCPPING镜像..."
docker build -t ${REGISTRY}/testtools-tcpping:${VERSION} -f Dockerfile.tcpping .
docker push ${REGISTRY}/testtools-tcpping:${VERSION}

echo "构建SKOOP镜像..."
docker build -t ${REGISTRY}/testtools-skoop:${VERSION} -f Dockerfile.skoop .
docker push ${REGISTRY}/testtools-skoop:${VERSION}

echo "构建IPERF镜像..."
docker build -t ${REGISTRY}/testtools-iperf:${VERSION} -f Dockerfile.iperf .
docker push ${REGISTRY}/testtools-iperf:${VERSION}

# 构建并推送控制器镜像
echo "构建控制器镜像..."
docker build -t ${REGISTRY}/testtools-controller:v86 .
docker push ${REGISTRY}/testtools-controller:v86

echo "所有镜像已构建并推送完成" 
