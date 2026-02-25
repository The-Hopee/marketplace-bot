FROM golang:1.21-alpine AS builder

WORKDIR /app

# Установка зависимостей для сборки
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bot ./cmd/bot

# Финальный образ с Chrome
FROM alpine:latest

WORKDIR /app

# Устанавливаем Chrome/Chromium и зависимости
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    font-noto-emoji \
    && rm -rf /var/cache/apk/*

# Переменные для Chrome
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/

# Копируем бинарник
COPY --from=builder /app/bot .

# Запуск
CMD ["./bot"]