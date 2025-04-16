#!/bin/bash
set -e

REGISTRY="172.30.1.13:18093"
VERSION="v1"

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

# 构建并推送控制器镜像
echo "构建控制器镜像..."
docker build -t ${REGISTRY}/testtools-controller:v60 .
docker push ${REGISTRY}/testtools-controller:v60

echo "所有镜像已构建并推送完成" 