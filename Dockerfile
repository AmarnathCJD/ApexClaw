FROM golang:1.25.3-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o apexclaw .

FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ffmpeg ca-certificates tzdata

COPY --from=builder /app/apexclaw .

COPY docker-entrypoint.sh .
RUN chmod +x docker-entrypoint.sh
RUN addgroup -g 1000 apexclaw && \
    adduser -D -u 1000 -G apexclaw apexclaw

RUN chown -R apexclaw:apexclaw /app

USER apexclaw
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD test -f /app/apexclaw || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
