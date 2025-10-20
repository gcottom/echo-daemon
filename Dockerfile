# Build a multi-stage Docker image with Go, Python, FFMPEG, and Chromium installed
FROM golang:latest AS builder
ENV CGO_ENABLED=0
WORKDIR /app
COPY ./server/go.mod ./server/go.sum ./
RUN go mod download
COPY ./server /app/server

WORKDIR /app/server/cmd
RUN go build -o server .

WORKDIR /app
# Install Python, Chromium, and FFMPEG in the final image
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

WORKDIR /app
COPY --from=builder /app/server/cmd/server /app/server
COPY ./python /app/python
RUN pip3 install -r /app/python/genre-service/requirements.txt && \
    pip3 install -r /app/python/music-api/requirements.txt
RUN pip3 install pyinstaller
# Use the correct script filenames (with hyphens) present in the repository
RUN pyinstaller --clean --onefile /app/python/genre-service/genre-service.py --name genre_service_bin --distpath /app
RUN pyinstaller --clean --onefile /app/python/music-api/music-api.py --name music_api_bin --distpath /app
ENTRYPOINT ["./server"]