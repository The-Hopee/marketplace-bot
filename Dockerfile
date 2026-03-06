# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bot ./cmd/bot

# Финальный образ с Chrome
FROM alpine:3.19

WORKDIR /app

# Устанавливаем Chromium и ВСЕ необходимые зависимости
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    font-noto \
    font-noto-emoji \
    font-noto-cjk \
    dbus \
    udev \
    && rm -rf /var/cache/apk/* /tmp/*

# Переменные для Chrome
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/

# Копируем бинарник
COPY --from=builder /app/bot .

CMD ["./bot"]