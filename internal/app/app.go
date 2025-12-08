package app

import (
	"log"
	"math/rand"
	"time"

	"telegram-secret-santa/config"
	"telegram-secret-santa/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Run() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	storage, err := service.NewStorage(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer storage.Close()

	bot, err := service.NewSecretSantaBot(cfg.Telegram.BotToken, cfg.Telegram.Admins, storage, cfg.TriggerWords)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	rand.Seed(time.Now().UnixNano())

	runBot(bot)
}

func runBot(bot *service.SecretSantaBot) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.Bot.GetUpdatesChan(u)

	log.Printf("Bot started and ready!")

	for update := range updates {
		if update.Message != nil {
			if update.Message.From != nil {
				bot.SaveUserInfo(update.Message.From)
			}
			if update.Message.Text != "" {
				bot.CheckTriggerWords(update.Message)
			}
			if update.Message.IsCommand() {
				bot.HandleCommand(update)
			} else if update.Message.ForwardFrom != nil {
				bot.HandleForwardedMessage(update.Message)
			}
		}
	}
}
