FROM golang:1.25.4-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o apexclaw .

FROM alpine:3.19
WORKDIR /app
RUN apk add --no-cache ffmpeg ca-certificates tzdata python3 py3-pip wkhtmltopdf poppler-utils ghostscript texlive-latex-base texlive-xetex imagemagick pandoc && \
    pip3 install --no-cache-dir yt-dlp --break-system-packages
COPY --from=builder /app/apexclaw .
RUN chmod +x apexclaw
ENTRYPOINT ["./apexclaw"]
