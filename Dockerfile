FROM golang:1.22-alpine AS builder
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
RUN chmod +x apexclaw
ENTRYPOINT ["./apexclaw"]
