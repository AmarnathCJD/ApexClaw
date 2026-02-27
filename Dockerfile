FROM golang:1.25.4-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o apexclaw .

FROM alpine:3.19
WORKDIR /app
RUN apk add --no-cache ffmpeg ca-certificates tzdata python3 py3-pip && \
    pip3 install --no-cache-dir yt-dlp
COPY --from=builder /app/apexclaw .
RUN chmod +x apexclaw
ENTRYPOINT ["./apexclaw"]
