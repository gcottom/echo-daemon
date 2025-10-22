# Multi-stage Dockerfile for echo-daemon with ONNX Runtime enabled by default
# Default target builds the server with ONNX support and includes Python-based helpers.

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

# ========== Runtime (Python + FFmpeg + Chromium + ORT) ==========
FROM python:3.10.14-bookworm
RUN apt-get update && apt-get install -y \
    ca-certificates curl gnupg ffmpeg \
    chromium \
    fonts-liberation fonts-noto-color-emoji \
    libnss3 libasound2 libgbm1 libx11-6 libx11-xcb1 libxcomposite1 \
    libxdamage1 libxrandr2 libgtk-3-0 libxss1 libxtst6 xdg-utils \
 && rm -rf /var/lib/apt/lists/*
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
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
# Copy server
COPY --from=builder /app/server/cmd/server /app/server
# Keep Python music_api_bin build only
COPY ./python /app/python
# Install music-api deps and also the conversion toolchain for musicnn -> ONNX
RUN pip3 install -r /app/python/music-api/requirements.txt && \
    pip3 install -r /app/python/scripts/requirements.txt
# Convert and vendor ONNX models into /app/models during build
ENV MUSICNN_ONNX_OUT=/app/models
RUN python3 /app/python/scripts/convert_musicnn_to_onnx.py || true
# Build the remaining Python binary
RUN pip3 install pyinstaller && \
    pyinstaller --clean --onefile /app/python/music-api/music-api.py --name music_api_bin --distpath /app
ENV ECHO_ONNX_DEBUG=1
ENTRYPOINT ["./server"]