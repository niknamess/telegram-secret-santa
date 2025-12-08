# Деплой на Railway

## Быстрый старт

1. **Создайте аккаунт на Railway**
   - Перейдите на https://railway.app
   - Войдите через GitHub

2. **Создайте новый проект**
   - Нажмите "New Project"
   - Выберите "Deploy from GitHub repo"
   - Выберите репозиторий `telegram-secret-santa`

3. **Добавьте Redis сервис**
   - В проекте нажмите "+ New"
   - Выберите "Database" → "Add Redis"
   - Railway автоматически создаст Redis и предоставит переменные окружения

4. **Настройте переменные окружения**
   В настройках вашего сервиса (бот) добавьте следующие переменные:
   
   ```
   TELEGRAM_BOT_TOKEN=ваш_токен_бота
   TELEGRAM_ADMINS=nikiname,username2
   REDIS_HOST=${{Redis.REDIS_HOST}}
   REDIS_PORT=${{Redis.REDIS_PORT}}
   REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}
   REDIS_DB=0
   TRIGGER_WORDS=жопа,мат,слово1
   ```
   
   **Важно:** 
   - `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` используйте переменные из Redis сервиса Railway
   - Railway автоматически предоставляет их в формате `${{ServiceName.VARIABLE_NAME}}`
   - Или используйте прямые значения из настроек Redis сервиса

5. **Деплой**
   - Railway автоматически начнет сборку и деплой
   - Проверьте логи в разделе "Deployments"

## Альтернативный способ (без Dockerfile)

Railway также может собрать Go приложение напрямую:

1. В настройках проекта выберите "Settings"
2. Измените "Build Command" на: `go build -o secret-santa-bot .`
3. Измените "Start Command" на: `./secret-santa-bot`

## Проверка работы

После деплоя проверьте:
- Логи в Railway Dashboard
- Отправьте `/start` боту в Telegram
- Бот должен ответить справкой

## Переменные окружения

| Переменная | Описание | Обязательная |
|------------|----------|--------------|
| `TELEGRAM_BOT_TOKEN` | Токен Telegram бота | ✅ Да |
| `TELEGRAM_ADMINS` | Список админов через запятую | Нет |
| `REDIS_HOST` | Хост Redis (из Railway Redis сервиса) | ✅ Да |
| `REDIS_PORT` | Порт Redis (из Railway Redis сервиса) | ✅ Да |
| `REDIS_PASSWORD` | Пароль Redis (из Railway Redis сервиса) | Нет |
| `REDIS_DB` | Номер БД Redis | Нет (по умолчанию 0) |
| `TRIGGER_WORDS` | Слова-триггеры через запятую | Нет |

## Troubleshooting

**Бот не запускается:**
- Проверьте, что все переменные окружения установлены
- Проверьте логи в Railway Dashboard
- Убедитесь, что Redis сервис запущен

**Ошибка подключения к Redis:**
- Убедитесь, что Redis сервис добавлен в проект
- Проверьте переменные `REDIS_HOST` и `REDIS_PORT`
- Используйте переменные из Redis сервиса Railway

**Бот не отвечает:**
- Проверьте, что токен бота правильный
- Проверьте логи на наличие ошибок
- Убедитесь, что бот запущен (статус "Running" в Railway)

