# 1. Сборка фронтенда
FROM node:20.18-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# 2. Сборка Go-бэкенда
FROM golang:1.23-alpine AS api
WORKDIR /src
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# 3. Итоговый образ
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=api /out/server ./server
COPY --from=web /web/dist ./frontend/dist
COPY data ./data
ENV ADDR=:8000 KB_DIR=/app/data/kb STATIC_DIR=/app/frontend/dist
USER app
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8000/api/health || exit 1
CMD ["./server"]
