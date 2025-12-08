# Настройка проекта на Railway

## После выполнения `railway link`

Проект уже связан с Railway. Теперь нужно настроить переменные окружения.

## Способ 1: Через Railway CLI

Если Railway CLI установлен и доступен:

```bash
# Установите переменные окружения
railway variables set TELEGRAM_BOT_TOKEN=ваш_токен_бота
railway variables set TELEGRAM_ADMINS=nikiname
railway variables set REDIS_DB=0
railway variables set TRIGGER_WORDS=жопа,мат

# Для Redis переменных (получите из Redis сервиса в Railway Dashboard):
railway variables set REDIS_HOST=<значение_из_redis_сервиса>
railway variables set REDIS_PORT=<значение_из_redis_сервиса>
railway variables set REDIS_PASSWORD=<значение_из_redis_сервиса>
```

Или используйте интерактивный скрипт:
```bash
./setup-railway.sh
```

## Способ 2: Через веб-интерфейс Railway (рекомендуется)

1. Откройте https://railway.app
2. Выберите ваш проект
3. Откройте сервис бота
4. Перейдите в "Variables"
5. Добавьте следующие переменные:

### Обязательные переменные:
- `TELEGRAM_BOT_TOKEN` - токен вашего бота от BotFather
- `TELEGRAM_ADMINS` - список админов через запятую (например: `nikiname`)

### Переменные Redis (из Redis сервиса):
1. Добавьте Redis сервис в проект (если еще не добавлен):
   - Нажмите "+ New" → "Database" → "Add Redis"

2. Откройте Redis сервис и скопируйте переменные:
   - `REDIS_HOST`
   - `REDIS_PORT`
   - `REDIS_PASSWORD`

3. Добавьте эти переменные в сервис бота

### Дополнительные переменные:
- `REDIS_DB` - номер БД Redis (по умолчанию: `0`)
- `TRIGGER_WORDS` - слова-триггеры через запятую (например: `жопа,мат`)

## Деплой

После настройки переменных Railway автоматически начнет деплой.

Проверьте логи:
- В Railway Dashboard → ваш сервис → "Deployments" → выберите последний деплой → "View Logs"

Или через CLI:
```bash
railway logs
```

## Проверка работы

1. Проверьте логи в Railway Dashboard
2. Отправьте `/start` боту в Telegram
3. Бот должен ответить справкой

## Troubleshooting

**Railway CLI не найден:**
- Перезапустите терминал
- Или используйте веб-интерфейс Railway

**Бот не запускается:**
- Проверьте все переменные окружения установлены
- Проверьте логи в Railway Dashboard
- Убедитесь, что Redis сервис запущен

**Ошибка подключения к Redis:**
- Убедитесь, что Redis сервис добавлен в проект
- Проверьте переменные REDIS_HOST, REDIS_PORT, REDIS_PASSWORD
- Используйте значения из Redis сервиса Railway

