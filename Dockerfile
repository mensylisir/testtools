# Build the manager binary
FROM golang:1.23 AS builder
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager main.go

# Use debian as base image to make sure we have dig (dnsutils)
FROM debian:bullseye-slim
RUN apt-get update && apt-get install -y dnsutils iproute2 curl iputils-ping && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /
COPY --from=builder /workspace/manager .

# Run as non-root user
USER 1000:1000

ENTRYPOINT ["/manager"] 