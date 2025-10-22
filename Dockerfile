# Multi-stage Dockerfile for echo-daemon with ONNX Runtime enabled by default

# ========== Builder (Go + ONNX Runtime dev) ==========
FROM golang:latest AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
# Prime module cache
COPY ./server/go.mod ./server/go.sum ./
RUN go mod download
# Bring in sources
COPY ./server /app/server
# ONNX Runtime version (used in this stage)
ARG ORT_VERSION=1.22.0
# Install ONNX Runtime dev files (headers + libs) matching architecture
RUN set -eux; \
    ARCH=$(dpkg --print-architecture); \
    case "$ARCH" in \
      amd64)  ORT_PKG="onnxruntime-linux-x64-${ORT_VERSION}.tgz" ;; \
      arm64)  ORT_PKG="onnxruntime-linux-aarch64-${ORT_VERSION}.tgz" ;; \
      *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/ort.tgz "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/${ORT_PKG}"; \
    mkdir -p /opt/onnxruntime; \
    tar -xzf /tmp/ort.tgz -C /opt/onnxruntime --strip-components=1; \
    cp -a /opt/onnxruntime/include/* /usr/local/include/; \
    cp -a /opt/onnxruntime/lib/* /usr/local/lib/; \
    rm -f /tmp/ort.tgz
# Build server with ONNX tag (CGO enabled)
WORKDIR /app/server/cmd
RUN CGO_ENABLED=1 go build -tags onnx -o server .

# ========== Runtime (Debian + FFmpeg + Chromium + ORT) ==========
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl ffmpeg \
    chromium \
    fonts-liberation fonts-noto-color-emoji \
    libnss3 libasound2 libgbm1 libx11-6 libx11-xcb1 libxcomposite1 \
    libxdamage1 libxrandr2 libgtk-3-0 libxss1 libxtst6 xdg-utils \
 && rm -rf /var/lib/apt/lists/*
# ONNX Runtime version (used in runtime stage)
ARG ORT_VERSION=1.22.0
# Install ORT runtime libs to match architecture
RUN set -eux; \
    ARCH=$(dpkg --print-architecture); \
    case "$ARCH" in \
      amd64)  ORT_PKG="onnxruntime-linux-x64-${ORT_VERSION}.tgz" ;; \
      arm64)  ORT_PKG="onnxruntime-linux-aarch64-${ORT_VERSION}.tgz" ;; \
      *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/ort.tgz "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/${ORT_PKG}"; \
    mkdir -p /opt/onnxruntime; \
    tar -xzf /tmp/ort.tgz -C /opt/onnxruntime --strip-components=1; \
    cp -a /opt/onnxruntime/lib/* /usr/local/lib/; \
    ldconfig 2>/dev/null || true; \
    rm -f /tmp/ort.tgz
ENV LD_LIBRARY_PATH=/usr/local/lib:${LD_LIBRARY_PATH}

WORKDIR /app
# Copy server binary
COPY --from=builder /app/server/cmd/server /app/server
# Copy pre-built ONNX models directly
COPY ./models/*.onnx /app/models/
ENV ECHO_ONNX_DEBUG=1
ENTRYPOINT ["./server"]