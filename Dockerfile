# syntax=docker/dockerfile:1
# ONNX-enabled builds need the Go build tag and a matching native CPU runtime.
FROM golang:latest AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates curl unzip && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY ./server/go.mod ./server/go.sum /app/server/
WORKDIR /app/server
RUN go mod download
COPY ./server /app/server
ARG ENABLE_ONNX=0
ARG ORT_VERSION=1.31.1
# The Go binding uses ONNX Runtime C API 31. Use the hash-pinned native library
# from Microsoft's official wheel; do not install Python or wheel dependencies.
RUN set -eu; \
    mkdir -p /ort-lib; \
    case "$ENABLE_ONNX" in \
      0) go build -o /echo-server ./cmd ;; \
      1) \
        test "$ORT_VERSION" = 1.31.1 || { echo "ONNX Go binding and hash-pinned native runtime require ORT_VERSION=1.31.1" >&2; exit 1; }; \
        case "$(dpkg --print-architecture)" in \
          amd64) asset='https://files.pythonhosted.org/packages/b4/1d/0f85ee31a3d93f2243a5cc29252becf858f53ac5cfa6c9c15d9ffe107eea/onnxruntime-1.31.1-cp311-cp311-manylinux_2_28_x86_64.whl'; sha='5752bd508126ae7c49894d3b9d2e83f0c3d487319246d04c444b393308d7ad7f' ;; \
          arm64) asset='https://files.pythonhosted.org/packages/02/ee/b173a49293b146b54bd057bf8681a015aef764d8f56d999ad8c6e53ab18d/onnxruntime-1.31.1-cp311-cp311-manylinux_2_28_aarch64.whl'; sha='da05378364c44d4407e463982d2bf8d5e4cfea873283757e48beeb41df97b7f3' ;; \
          *) echo "Unsupported ONNX platform" >&2; exit 1 ;; \
        esac; \
        curl --fail --silent --show-error --location --retry 2 --max-time 120 "$asset" -o /tmp/ort.whl; \
        printf '%s  %s\n' "$sha" /tmp/ort.whl | sha256sum -c -; \
        unzip -j /tmp/ort.whl onnxruntime/capi/libonnxruntime.so.1.31.1 onnxruntime/capi/libonnxruntime_providers_shared.so -d /ort-lib; \
        ln -s libonnxruntime.so.1.31.1 /ort-lib/libonnxruntime.so; \
        rm /tmp/ort.whl; \
        CGO_ENABLED=1 go build -tags onnx -o /echo-server ./cmd ;; \
      *) echo "ENABLE_ONNX must be 0 or 1" >&2; exit 1 ;; \
    esac

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl ffmpeg libstdc++6 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /ort-lib/ /usr/local/lib/
RUN ldconfig
WORKDIR /app
COPY --from=builder /echo-server /app/server
COPY ./models/*.onnx /app/models/
COPY ./models/LICENSE.musicnn ./models/README.md ./models/SHA256SUMS /app/models/
ENTRYPOINT ["./server"]
