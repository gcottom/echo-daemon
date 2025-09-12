FROM golang:latest AS builder
ENV CGO_ENABLED=0
WORKDIR /app
COPY ./server/go.mod ./server/go.sum ./
RUN go mod download
COPY ./server /app/server

WORKDIR /app/server/cmd
RUN go build -o server .

WORKDIR /app
FROM python:3.10.14
RUN apt-get update && apt-get install -y ffmpeg

WORKDIR /app
COPY --from=builder /app/server/cmd/server /app/server
COPY ./python /app/python
RUN pip3 install -r /app/python/genre-service/requirements.txt && \
    pip3 install -r /app/python/music-api/requirements.txt

ENTRYPOINT ["./server"]