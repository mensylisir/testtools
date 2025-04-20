# PowerShell脚本用于在Windows环境下构建Docker镜像

$REGISTRY = "registry.dev.rdev.tech:18093"
$VERSION = "v1"

# 构建并推送FIO镜像
Write-Host "构建FIO镜像..."
docker build -t "${REGISTRY}/testtools-fio:${VERSION}" -f Dockerfile.fio .
docker push "${REGISTRY}/testtools-fio:${VERSION}"

# 构建并推送DIG镜像
Write-Host "构建DIG镜像..."
docker build -t "${REGISTRY}/testtools-dig:${VERSION}" -f Dockerfile.dig .
docker push "${REGISTRY}/testtools-dig:${VERSION}"

# 构建并推送PING镜像
Write-Host "构建PING镜像..."
docker build -t "${REGISTRY}/testtools-ping:${VERSION}" -f Dockerfile.ping .
docker push "${REGISTRY}/testtools-ping:${VERSION}"

Write-Host "构建NC镜像..."
docker build -t ${REGISTRY}/testtools-nc:${VERSION} -f Dockerfile.nc .
docker push ${REGISTRY}/testtools-nc:${VERSION}

Write-Host "构建TCPPING镜像..."
docker build -t ${REGISTRY}/testtools-tcpping:${VERSION} -f Dockerfile.tcpping .
docker push ${REGISTRY}/testtools-tcpping:${VERSION}

Write-Host "构建SKOOP镜像..."
docker build -t ${REGISTRY}/testtools-skoop:${VERSION} -f Dockerfile.skoop .
docker push ${REGISTRY}/testtools-skoop:${VERSION}

Write-Host "构建IPERF镜像..."
docker build -t ${REGISTRY}/testtools-iperf:${VERSION} -f Dockerfile.iperf .
docker push ${REGISTRY}/testtools-iperf:${VERSION}

# 构建并推送控制器镜像
Write-Host "构建控制器镜像..."
docker build -t "${REGISTRY}/testtools-controller:v70" .
docker push "${REGISTRY}/testtools-controller:v70"

Write-Host "所有镜像已构建并推送完成" 