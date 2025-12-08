#!/bin/bash

echo "🚂 Настройка переменных окружения для Railway"
echo ""

# Проверка Railway CLI
RAILWAY_CMD=""
if command -v railway &> /dev/null; then
    RAILWAY_CMD="railway"
elif [ -f ~/.railway/bin/railway ]; then
    RAILWAY_CMD="$HOME/.railway/bin/railway"
elif [ -f /usr/local/bin/railway ]; then
    RAILWAY_CMD="/usr/local/bin/railway"
else
    echo "⚠️  Railway CLI не найден"
    echo ""
    echo "Попробуйте найти railway командой:"
    echo "  which railway"
    echo "  find ~ -name railway 2>/dev/null"
    echo ""
    echo "Или используйте веб-интерфейс Railway для настройки переменных"
    echo "https://railway.app"
    exit 1
fi

echo "✅ Railway CLI найден: $RAILWAY_CMD"
echo ""

# Чтение переменных из .env
if [ -f .env ]; then
    echo "📖 Читаю переменные из .env файла..."
    source .env
else
    echo "⚠️  .env файл не найден, используем значения по умолчанию"
fi

# Установка переменных
echo "📝 Установка переменных окружения в Railway..."
echo ""

if [ -n "$TELEGRAM_BOT_TOKEN" ] && [ "$TELEGRAM_BOT_TOKEN" != "YOUR_BOT_TOKEN_HERE" ]; then
    echo "✅ Устанавливаю TELEGRAM_BOT_TOKEN..."
    $RAILWAY_CMD variables set TELEGRAM_BOT_TOKEN="$TELEGRAM_BOT_TOKEN"
else
    echo "⚠️  TELEGRAM_BOT_TOKEN не установлен в .env"
    read -p "Введите TELEGRAM_BOT_TOKEN: " TELEGRAM_BOT_TOKEN
    $RAILWAY_CMD variables set TELEGRAM_BOT_TOKEN="$TELEGRAM_BOT_TOKEN"
fi

if [ -n "$TELEGRAM_ADMINS" ]; then
    echo "✅ Устанавливаю TELEGRAM_ADMINS..."
    $RAILWAY_CMD variables set TELEGRAM_ADMINS="$TELEGRAM_ADMINS"
else
    echo "⚠️  TELEGRAM_ADMINS не установлен в .env"
    read -p "Введите TELEGRAM_ADMINS (через запятую): " TELEGRAM_ADMINS
    $RAILWAY_CMD variables set TELEGRAM_ADMINS="$TELEGRAM_ADMINS"
fi

if [ -n "$REDIS_DB" ]; then
    echo "✅ Устанавливаю REDIS_DB..."
    $RAILWAY_CMD variables set REDIS_DB="$REDIS_DB"
else
    echo "✅ Устанавливаю REDIS_DB=0 (по умолчанию)..."
    $RAILWAY_CMD variables set REDIS_DB="0"
fi

if [ -n "$TRIGGER_WORDS" ]; then
    echo "✅ Устанавливаю TRIGGER_WORDS..."
    $RAILWAY_CMD variables set TRIGGER_WORDS="$TRIGGER_WORDS"
else
    echo "ℹ️  TRIGGER_WORDS не установлен (опционально)"
    read -p "Введите TRIGGER_WORDS (через запятую, или Enter чтобы пропустить): " TRIGGER_WORDS
    if [ -n "$TRIGGER_WORDS" ]; then
        $RAILWAY_CMD variables set TRIGGER_WORDS="$TRIGGER_WORDS"
    fi
fi

echo ""
echo "✅ Основные переменные установлены!"
echo ""
echo "⚠️  ВАЖНО: Redis переменные (REDIS_HOST, REDIS_PORT, REDIS_PASSWORD)"
echo "   нужно установить вручную из Redis сервиса Railway"
echo ""
echo "Инструкция:"
echo "1. Откройте Railway Dashboard: https://railway.app"
echo "2. Найдите Redis сервис в вашем проекте"
echo "3. Скопируйте значения REDIS_HOST, REDIS_PORT, REDIS_PASSWORD"
echo "4. Установите их командой:"
echo "   $RAILWAY_CMD variables set REDIS_HOST=<значение>"
echo "   $RAILWAY_CMD variables set REDIS_PORT=<значение>"
echo "   $RAILWAY_CMD variables set REDIS_PASSWORD=<значение>"
echo ""
echo "Или используйте веб-интерфейс Railway для настройки Redis переменных"

