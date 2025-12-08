#!/bin/bash

echo "🚂 Настройка проекта для Railway"
echo ""

# Проверка Railway CLI
if ! command -v railway &> /dev/null; then
    echo "⚠️  Railway CLI не найден в PATH"
    echo "Попробуйте перезапустить терминал или добавить Railway в PATH"
    echo ""
    echo "Или используйте веб-интерфейс Railway для настройки переменных"
    exit 1
fi

echo "✅ Railway CLI найден"
echo ""

# Установка переменных окружения
echo "📝 Установка переменных окружения..."
echo ""

read -p "Введите TELEGRAM_BOT_TOKEN: " BOT_TOKEN
railway variables set TELEGRAM_BOT_TOKEN="$BOT_TOKEN"

read -p "Введите TELEGRAM_ADMINS (через запятую, например: nikiname): " ADMINS
railway variables set TELEGRAM_ADMINS="$ADMINS"

read -p "Введите REDIS_DB (по умолчанию 0): " REDIS_DB
REDIS_DB=${REDIS_DB:-0}
railway variables set REDIS_DB="$REDIS_DB"

read -p "Введите TRIGGER_WORDS (через запятую, например: жопа,мат): " TRIGGER_WORDS
railway variables set TRIGGER_WORDS="$TRIGGER_WORDS"

echo ""
echo "✅ Переменные окружения установлены!"
echo ""
echo "⚠️  Важно: REDIS_HOST, REDIS_PORT, REDIS_PASSWORD"
echo "   должны быть установлены из Redis сервиса Railway"
echo ""
echo "Для этого:"
echo "1. Откройте Railway Dashboard"
echo "2. Найдите Redis сервис в проекте"
echo "3. Скопируйте переменные REDIS_HOST, REDIS_PORT, REDIS_PASSWORD"
echo "4. Установите их командой:"
echo "   railway variables set REDIS_HOST=<значение>"
echo "   railway variables set REDIS_PORT=<значение>"
echo "   railway variables set REDIS_PASSWORD=<значение>"
echo ""
echo "Или используйте веб-интерфейс Railway для настройки"

