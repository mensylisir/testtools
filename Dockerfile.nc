FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    netcat-openbsd \
    procps \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

# 设置工作目录
WORKDIR /app

CMD ["bash"] 