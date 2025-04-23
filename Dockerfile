# --- Этап сборки ---
    FROM golang:1.24-alpine AS builder

    # Устанавливаем рабочую директорию внутри контейнера
    WORKDIR /build
    
    # Копируем файлы управления зависимостями
    COPY go.mod go.sum ./
    # Скачиваем зависимости (этот слой будет кешироваться)
    RUN go mod download
    
    # Копируем исходный код проекта
    COPY . .
    
    # Устанавливаем GIN_MODE=release для сборки (если это влияет на сборку, обычно нет)
    # ENV GIN_MODE=release
    
    # Собираем приложение.
    # -ldflags="-w -s" уменьшает размер бинарника (удаляет отладочную информацию)
    # CGO_ENABLED=0 создает статически скомпилированный бинарник без C-зависимостей (важно для Alpine)
    RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /imagecleaner ./cmd/imagecleaner/main.go
    
    
    # --- Этап выполнения ---
    FROM alpine:latest
    
    # Устанавливаем рабочую директорию
    WORKDIR /app
    
    # Копируем ТОЛЬКО скомпилированный бинарник из этапа сборки
    COPY --from=builder /imagecleaner /app/imagecleaner
    
    # Копируем статические файлы и шаблоны
    COPY web/templates /app/web/templates
    COPY web/static /app/web/static
    
    # Создаем папку для загрузок ВНУТРИ контейнера (данные будут в volume)
    # RUN mkdir -p /app/uploads && chown nobody:nogroup /app/uploads
    # Создаем папку для БД ВНУТРИ контейнера (данные будут в volume)
    # RUN mkdir -p /app/db && chown nobody:nogroup /app/db
    # Примечание: Volumes в docker-compose сами создадут папки, если их нет.
    # chown нужен, если будем запускать от имени non-root user.
    
    # Указываем порт, который слушает наше приложение внутри контейнера
    EXPOSE 8080
    
    # Пользователь без прав root (рекомендуется)
    # Создадим пользователя и группу 'appuser'
    # RUN addgroup -S appgroup && adduser -S appuser -G appgroup
    # USER appuser
    
    # Команда для запуска приложения при старте контейнера
    # Путь к БД и папке uploads будут определяться относительно WORKDIR /app
    CMD ["/app/imagecleaner"]