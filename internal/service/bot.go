package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"

	"telegram-secret-santa/internal/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func escapeMarkdown(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "_", "\\_")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	text = strings.ReplaceAll(text, "~", "\\~")
	text = strings.ReplaceAll(text, "`", "\\`")
	text = strings.ReplaceAll(text, ">", "\\>")
	text = strings.ReplaceAll(text, "#", "\\#")
	text = strings.ReplaceAll(text, "+", "\\+")
	text = strings.ReplaceAll(text, "-", "\\-")
	text = strings.ReplaceAll(text, "=", "\\=")
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "{", "\\{")
	text = strings.ReplaceAll(text, "}", "\\}")
	text = strings.ReplaceAll(text, ".", "\\.")
	text = strings.ReplaceAll(text, "!", "\\!")
	return text
}

type SecretSantaBot struct {
	Bot          *tgbotapi.BotAPI
	Storage      domain.StorageInterface
	Admins       map[string]bool
	TriggerWords []string
	UserTriggers map[int64][]string
}

func NewSecretSantaBot(token string, admins []string, storage domain.StorageInterface, triggerWords []string) (*SecretSantaBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	adminMap := make(map[string]bool)
	for _, admin := range admins {
		adminUsername := strings.TrimPrefix(admin, "@")
		adminMap[strings.ToLower(adminUsername)] = true
	}

	return &SecretSantaBot{
		Bot:          bot,
		Storage:      storage,
		Admins:       adminMap,
		TriggerWords: triggerWords,
		UserTriggers: make(map[int64][]string),
	}, nil
}

func (s *SecretSantaBot) IsAdmin(username string) bool {
	if username == "" {
		return false
	}
	usernameLower := strings.ToLower(strings.TrimPrefix(username, "@"))
	return s.Admins[usernameLower]
}

func (s *SecretSantaBot) AddParticipant(userID int64, username, fullName string) error {
	p := &domain.Participant{
		UserID:   userID,
		Username: username,
		FullName: fullName,
	}
	return s.Storage.SaveParticipant(p)
}

func (s *SecretSantaBot) SaveUserInfo(user *tgbotapi.User) {
	if user == nil || user.ID == 0 {
		return
	}
	fullName := user.FirstName
	if user.LastName != "" {
		fullName += " " + user.LastName
	}
	if user.UserName != "" {
		existing, _ := s.Storage.GetParticipant(user.ID)
		if existing == nil {
			s.AddParticipant(user.ID, user.UserName, fullName)
			log.Printf("SaveUserInfo: saved user info userID=%d, username=%s, fullName=%s", user.ID, user.UserName, fullName)
		} else {
			if existing.Username != user.UserName || existing.FullName != fullName {
				existing.Username = user.UserName
				existing.FullName = fullName
				s.Storage.SaveParticipant(existing)
				log.Printf("SaveUserInfo: updated user info userID=%d, username=%s, fullName=%s", user.ID, user.UserName, fullName)
			}
		}
	} else {
		log.Printf("SaveUserInfo: user userID=%d has no username, skipping", user.ID)
	}
}

func (s *SecretSantaBot) RemoveParticipant(userID int64) error {
	if err := s.Storage.DeleteParticipant(userID); err != nil {
		return err
	}

	if err := s.Storage.DeleteAllRestrictionsForUser(userID); err != nil {
		return err
	}

	if err := s.Storage.DeleteAssignment(userID); err != nil {
		return err
	}

	restrictions, _, err := s.Storage.GetAllRestrictions()
	if err != nil {
		return err
	}

	for otherUserID, userRestrictions := range restrictions {
		for forbiddenID := range userRestrictions {
			if forbiddenID == userID {
				if err := s.Storage.DeleteRestriction(otherUserID, userID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *SecretSantaBot) AddRestriction(userID, forbiddenUserID, creatorID int64) error {
	log.Printf("AddRestriction: saving to Redis - userID=%d, forbiddenUserID=%d, creatorID=%d", userID, forbiddenUserID, creatorID)
	err := s.Storage.SaveRestriction(userID, forbiddenUserID, creatorID)
	if err != nil {
		log.Printf("AddRestriction: failed to save to Redis: %v", err)
		return err
	}
	log.Printf("AddRestriction: successfully saved to Redis")
	return nil
}

func (s *SecretSantaBot) RemoveRestriction(userID, forbiddenUserID int64) error {
	log.Printf("RemoveRestriction: deleting from Redis - userID=%d, forbiddenUserID=%d", userID, forbiddenUserID)
	err := s.Storage.DeleteRestriction(userID, forbiddenUserID)
	if err != nil {
		log.Printf("RemoveRestriction: failed to delete from Redis: %v", err)
		return err
	}
	log.Printf("RemoveRestriction: successfully deleted from Redis")
	return nil
}

func (s *SecretSantaBot) GenerateAssignments() error {
	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}

	if len(participants) < 2 {
		return fmt.Errorf("at least 2 participants required")
	}

	restrictions, _, err := s.Storage.GetAllRestrictions()
	if err != nil {
		return fmt.Errorf("failed to get restrictions: %w", err)
	}
	log.Printf("GenerateAssignments: loaded %d user restrictions from Redis", len(restrictions))
	totalRestrictions := 0
	for _, userRestrictions := range restrictions {
		totalRestrictions += len(userRestrictions)
	}
	log.Printf("GenerateAssignments: total restrictions count: %d", totalRestrictions)

	participantIDs := make([]int64, 0, len(participants))
	for id := range participants {
		participantIDs = append(participantIDs, id)
	}

	maxAttempts := 1000
	for attempt := 0; attempt < maxAttempts; attempt++ {
		receivers := make([]int64, len(participantIDs))
		copy(receivers, participantIDs)
		rand.Shuffle(len(receivers), func(i, j int) {
			receivers[i], receivers[j] = receivers[j], receivers[i]
		})

		valid := true
		assignments := make(map[int64]int64)

		for i, giverID := range participantIDs {
			receiverID := receivers[i]

			if giverID == receiverID {
				valid = false
				break
			}

			if userRestrictions, ok := restrictions[giverID]; ok && userRestrictions[receiverID] {
				valid = false
				break
			}

			assignments[giverID] = receiverID
		}

		if valid {
			log.Printf("GenerateAssignments: valid assignment found on attempt %d", attempt+1)

			var logBuilder strings.Builder
			logBuilder.WriteString("\n")
			logBuilder.WriteString("═══════════════════════════════════════════════════════════\n")
			logBuilder.WriteString("🎅 РАСПРЕДЕЛЕНИЕ ТАЙНОГО САНТЫ 🎅\n")
			logBuilder.WriteString("═══════════════════════════════════════════════════════════\n")
			logBuilder.WriteString("\n")

			for giverID, receiverID := range assignments {
				if err := s.Storage.SaveAssignment(giverID, receiverID); err != nil {
					return fmt.Errorf("failed to save assignment: %w", err)
				}

				giver, err := s.Storage.GetParticipant(giverID)
				receiver, err2 := s.Storage.GetParticipant(receiverID)
				if err == nil && err2 == nil && giver != nil && receiver != nil {
					giverName := giver.FullName
					if giver.Username != "" {
						giverName += " (@" + giver.Username + ")"
					}
					receiverName := receiver.FullName
					if receiver.Username != "" {
						receiverName += " (@" + receiver.Username + ")"
					}
					logBuilder.WriteString(fmt.Sprintf("  🎁 %s\n", giverName))
					logBuilder.WriteString(fmt.Sprintf("     └─> дарит подарок: %s\n", receiverName))

					receiverWish, err3 := s.Storage.GetWish(receiverID)
					if err3 == nil && receiverWish != "" {
						logBuilder.WriteString(fmt.Sprintf("        💝 Желание: %s\n", receiverWish))
					} else {
						logBuilder.WriteString("        💝 Желание: не указано\n")
					}

					comments, err4 := s.Storage.GetComments(receiverID)
					if err4 == nil && len(comments) > 0 {
						logBuilder.WriteString("        💬 Комментарии от участников:\n")
						allParticipants, _ := s.Storage.GetAllParticipants()
						for authorID, comment := range comments {
							author, ok := allParticipants[authorID]
							if ok && author != nil {
								authorName := author.FullName
								if author.Username != "" {
									authorName += " (@" + author.Username + ")"
								}
								logBuilder.WriteString(fmt.Sprintf("          👤 %s: %s\n", authorName, comment))
							} else {
								logBuilder.WriteString(fmt.Sprintf("          👤 Участник (ID: %d): %s\n", authorID, comment))
							}
						}
					}

					logBuilder.WriteString(fmt.Sprintf("        (ID: %d -> %d)\n", giverID, receiverID))
					logBuilder.WriteString("\n")
				} else {
					logBuilder.WriteString(fmt.Sprintf("  🎁 userID:%d\n", giverID))
					logBuilder.WriteString(fmt.Sprintf("     └─> дарит подарок: userID:%d\n", receiverID))

					receiverWish, err3 := s.Storage.GetWish(receiverID)
					if err3 == nil && receiverWish != "" {
						logBuilder.WriteString(fmt.Sprintf("        💝 Желание: %s\n", receiverWish))
					} else {
						logBuilder.WriteString("        💝 Желание: не указано\n")
					}

					comments, err4 := s.Storage.GetComments(receiverID)
					if err4 == nil && len(comments) > 0 {
						logBuilder.WriteString("        💬 Комментарии от участников:\n")
						allParticipants, _ := s.Storage.GetAllParticipants()
						for authorID, comment := range comments {
							author, ok := allParticipants[authorID]
							if ok && author != nil {
								authorName := author.FullName
								if author.Username != "" {
									authorName += " (@" + author.Username + ")"
								}
								logBuilder.WriteString(fmt.Sprintf("          👤 %s: %s\n", authorName, comment))
							} else {
								logBuilder.WriteString(fmt.Sprintf("          👤 Участник (ID: %d): %s\n", authorID, comment))
							}
						}
					}

					logBuilder.WriteString("\n")
				}
			}

			logBuilder.WriteString("═══════════════════════════════════════════════════════════\n")
			logBuilder.WriteString("✅ Все назначения успешно сохранены в Redis\n")
			logBuilder.WriteString("═══════════════════════════════════════════════════════════\n")

			log.Printf("%s", logBuilder.String())
			return nil
		}
	}

	return fmt.Errorf("failed to create assignment with current restrictions")
}

func (s *SecretSantaBot) SendAssignment(userID int64) error {
	receiverID, err := s.Storage.GetAssignment(userID)
	if err != nil {
		return fmt.Errorf("failed to get assignment: %w", err)
	}
	if receiverID == 0 {
		return fmt.Errorf("assignment not found")
	}

	receiver, err := s.Storage.GetParticipant(receiverID)
	if err != nil {
		return fmt.Errorf("failed to get participant: %w", err)
	}
	if receiver == nil {
		return fmt.Errorf("participant not found")
	}

	message := fmt.Sprintf("🎅 Тайный Санта назначен!\n\n"+
		"Вы дарите подарок: %s", receiver.FullName)

	if receiver.Username != "" {
		message += fmt.Sprintf(" (@%s)", receiver.Username)
	}

	receiverWish, err := s.Storage.GetWish(receiverID)
	if err == nil && receiverWish != "" {
		message += fmt.Sprintf("\n\n💝 Желание получателя:\n%s", receiverWish)
		log.Printf("SendAssignment: sending message to userID=%d with wish for receiverID=%d", userID, receiverID)
	} else {
		log.Printf("SendAssignment: sending message to userID=%d without wish (receiverID=%d has no wish)", userID, receiverID)
	}

	comments, err := s.Storage.GetComments(receiverID)
	if err == nil && len(comments) > 0 {
		message += "\n\n💬 Комментарии от участников:"
		participants, _ := s.Storage.GetAllParticipants()
		for authorID, comment := range comments {
			author, ok := participants[authorID]
			if ok && author != nil {
				authorName := author.FullName
				if author.Username != "" {
					authorName += " (@" + author.Username + ")"
				}
				message += fmt.Sprintf("\n\n👤 %s:\n%s", authorName, comment)
			} else {
				message += fmt.Sprintf("\n\n👤 Участник (ID: %d):\n%s", authorID, comment)
			}
		}
		log.Printf("SendAssignment: sending message to userID=%d with %d comments for receiverID=%d", userID, len(comments), receiverID)
	}

	msg := tgbotapi.NewMessage(userID, message)
	_, err = s.Bot.Send(msg)
	if err != nil {
		log.Printf("SendAssignment: failed to send message to userID=%d: %v", userID, err)
	} else {
		log.Printf("SendAssignment: successfully sent message to userID=%d", userID)
	}
	return err
}

func (s *SecretSantaBot) HandleCommand(update tgbotapi.Update) {
	msg := update.Message
	if msg != nil && msg.From != nil {
		s.SaveUserInfo(msg.From)
	}
	command := strings.ToLower(msg.Command())

	switch command {
	case "start", "help":
		s.sendHelpMessage(msg)

	case "startgame":
		s.handleSendAssignments(msg)

	case "add":
		s.handleAddParticipant(msg)

	case "adduser":
		s.handleAddUserByUsername(msg)

	case "remove":
		s.handleRemoveParticipant(msg)

	case "list":
		s.handleListParticipants(msg)

	case "restrict":
		s.handleAddRestriction(msg)

	case "unrestrict":
		s.handleRemoveRestriction(msg)

	case "restrictions":
		s.handleListRestrictions(msg)

	case "generate":
		s.handleGenerate(msg)

	case "send":
		s.handleSendAssignments(msg)

	case "reset":
		s.handleReset(msg)

	case "status":
		s.handleStatus(msg)

	case "members":
		s.handleMembersCount(msg)

	case "wish":
		s.handleSetWish(msg)

	case "mywish":
		s.handleGetWish(msg)

	case "deletewish":
		s.handleDeleteWish(msg)

	case "addtrigger":
		s.handleAddTrigger(msg)

	case "addtriggermessage":
		s.handleAddTriggerMessage(msg)

	case "comment":
		s.handleAddComment(msg)

	default:
		s.sendMessage(msg.Chat.ID, "Неизвестная команда. Используйте /help для списка команд.")
	}
}

func (s *SecretSantaBot) sendHelpMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	username := msg.From.UserName
	isAdmin := s.IsAdmin(username)

	var helpText string
	if isAdmin {
		helpText = `🎅 *Бот для Тайного Санты*

*Команды для всех:*

/add - Добавить себя в игру
/adduser @username - Добавить участника по username (в группах - через упоминание, в личке - перешлите сообщение от пользователя)
/remove - Удалить себя из игры
/list - Список участников
/restrict @username - Добавить ограничение (вы не получите этого человека)
/unrestrict @username - Удалить ограничение (только свои или админ может удалять любые)
/restrictions - Показать все ограничения
/status - Показать статус игры
/members - Показать количество участников в группе (только в группах)
/wish текст - Указать или изменить желание (что вы хотите получить от тайного санта)
/mywish - Показать ваше текущее желание
/deletewish - Удалить ваше желание
/addtrigger слово - Добавить слово-триггер (при упоминании этого слова бот отправит специальное сообщение)
/addtriggermessage слово|сообщение - Добавить сообщение к триггерному слову (сообщения выбираются случайно)
/comment @username текст - Добавить комментарий/подсказку для участника (что нужно дарить)

*Команды для администраторов:*

/generate - Сгенерировать распределение
/startgame или /send - Начать игру (отправить всем участникам их получателей)
/reset - Сбросить игру

*Пример использования:*
1. Участники добавляются через /add
2. Устанавливаются ограничения через /restrict @username
3. Участники указывают желания через /wish
4. Администратор генерирует распределение через /generate
5. Администратор начинает игру через /startgame`
	} else {
		helpText = `🎅 *Бот для Тайного Санты*

*Команды:*

/add - Добавить себя в игру
/adduser @username - Добавить участника по username (в группах - через упоминание, в личке - перешлите сообщение от пользователя)
/remove - Удалить себя из игры
/list - Список участников
/restrict @username - Добавить ограничение (вы не получите этого человека)
/unrestrict @username - Удалить ограничение (только свои или админ может удалять любые)
/restrictions - Показать все ограничения
/status - Показать статус игры
/members - Показать количество участников в группе (только в группах)
/wish текст - Указать или изменить желание (что вы хотите получить от тайного санта)
/mywish - Показать ваше текущее желание
/deletewish - Удалить ваше желание
/addtrigger слово - Добавить слово-триггер (при упоминании этого слова бот отправит специальное сообщение)
/addtriggermessage слово|сообщение - Добавить сообщение к триггерному слову (сообщения выбираются случайно)
/comment @username текст - Добавить комментарий/подсказку для участника (что нужно дарить)

*Пример использования:*
1. Участники добавляются через /add
2. Устанавливаются ограничения через /restrict @username
3. Участники указывают желания через /wish
4. Администратор генерирует распределение
5. Администратор начинает игру`
	}

	response := tgbotapi.NewMessage(chatID, helpText)
	response.ParseMode = "Markdown"
	_, err := s.Bot.Send(response)
	if err != nil {
		log.Printf("Failed to send help message: %v", err)
		responsePlain := tgbotapi.NewMessage(chatID, helpText)
		responsePlain.ParseMode = ""
		s.Bot.Send(responsePlain)
	}
}

func (s *SecretSantaBot) handleAddParticipant(msg *tgbotapi.Message) {
	userID := msg.From.ID
	username := msg.From.UserName
	fullName := msg.From.FirstName
	if msg.From.LastName != "" {
		fullName += " " + msg.From.LastName
	}

	if err := s.AddParticipant(userID, username, fullName); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при добавлении: %v", err))
		return
	}
	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Вы добавлены в игру, %s!", fullName))
}

func (s *SecretSantaBot) handleAddUserByUsername(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.CommandArguments())
	if text == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите username пользователя. Пример: /adduser @username")
		return
	}

	username := strings.TrimPrefix(text, "@")
	log.Printf("handleAddUserByUsername: searching for username=%s, chatID=%d, isGroup=%v", username, msg.Chat.ID, msg.Chat.IsGroup() || msg.Chat.IsSuperGroup())

	var targetUser *tgbotapi.User
	var hasTextMention bool

	if len(msg.Entities) > 0 {
		log.Printf("handleAddUserByUsername: found %d entities", len(msg.Entities))
		for i, entity := range msg.Entities {
			log.Printf("handleAddUserByUsername: entity[%d] type=%s, offset=%d, length=%d", i, entity.Type, entity.Offset, entity.Length)
			if entity.Type == "text_mention" && entity.User != nil {
				hasTextMention = true
				log.Printf("handleAddUserByUsername: text_mention found, userID=%d, username=%s", entity.User.ID, entity.User.UserName)
				if entity.User.UserName == username || strings.EqualFold(entity.User.UserName, username) || username == "" {
					targetUser = entity.User
					log.Printf("handleAddUserByUsername: matched user via text_mention: userID=%d, username=%s", targetUser.ID, targetUser.UserName)
					break
				}
			}
			if entity.Type == "mention" {
				mentionText := msg.Text[entity.Offset : entity.Offset+entity.Length]
				log.Printf("handleAddUserByUsername: mention found, text=%s", mentionText)
			}
		}
		if hasTextMention && targetUser == nil {
			log.Printf("handleAddUserByUsername: text_mention found but username doesn't match, using text_mention user anyway")
			for _, entity := range msg.Entities {
				if entity.Type == "text_mention" && entity.User != nil {
					targetUser = entity.User
					log.Printf("handleAddUserByUsername: using text_mention user: userID=%d, username=%s", targetUser.ID, targetUser.UserName)
					break
				}
			}
		}
	} else {
		log.Printf("handleAddUserByUsername: no entities found in message")
	}

	if targetUser == nil {
		log.Printf("handleAddUserByUsername: trying to find user in saved participants by username=%s", username)
		allParticipants, err := s.Storage.GetAllParticipants()
		if err != nil {
			log.Printf("handleAddUserByUsername: failed to get saved participants: %v", err)
		} else {
			log.Printf("handleAddUserByUsername: checking %d saved participants", len(allParticipants))
			for userID, participant := range allParticipants {
				log.Printf("handleAddUserByUsername: participant userID=%d, username=%s, comparing with %s", userID, participant.Username, username)
				if strings.EqualFold(participant.Username, username) {
					log.Printf("handleAddUserByUsername: found user in saved participants userID=%d, username=%s", userID, participant.Username)
					targetUser = &tgbotapi.User{
						ID:        userID,
						UserName:  participant.Username,
						FirstName: strings.Fields(participant.FullName)[0],
					}
					if len(strings.Fields(participant.FullName)) > 1 {
						targetUser.LastName = strings.Join(strings.Fields(participant.FullName)[1:], " ")
					}
					break
				}
			}
			if targetUser == nil {
				log.Printf("handleAddUserByUsername: user @%s not found in %d saved participants", username, len(allParticipants))
			}
		}
	}

	if targetUser == nil && (msg.Chat.IsGroup() || msg.Chat.IsSuperGroup()) {
		log.Printf("handleAddUserByUsername: trying to find user in administrators, chatID=%d", msg.Chat.ID)
		admins, err := s.Bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
			ChatConfig: tgbotapi.ChatConfig{ChatID: msg.Chat.ID},
		})
		if err != nil {
			log.Printf("handleAddUserByUsername: failed to get administrators: %v", err)
		} else {
			log.Printf("handleAddUserByUsername: found %d administrators", len(admins))
			for i, admin := range admins {
				if admin.User != nil {
					log.Printf("handleAddUserByUsername: admin[%d] userID=%d, username=%s", i, admin.User.ID, admin.User.UserName)
					if admin.User.UserName == username || strings.EqualFold(admin.User.UserName, username) {
						targetUser = admin.User
						log.Printf("handleAddUserByUsername: matched user via administrators: userID=%d, username=%s", targetUser.ID, targetUser.UserName)
						break
					}
				}
			}
		}
	}

	if targetUser != nil {
		log.Printf("handleAddUserByUsername: user found, userID=%d, username=%s, checking if already participant", targetUser.ID, targetUser.UserName)
		existing, err := s.Storage.GetParticipant(targetUser.ID)
		if err == nil && existing != nil {
			log.Printf("handleAddUserByUsername: user already exists as participant")
			s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s уже участвует в игре.", username))
			return
		}

		fullName := targetUser.FirstName
		if targetUser.LastName != "" {
			fullName += " " + targetUser.LastName
		}
		log.Printf("handleAddUserByUsername: adding participant userID=%d, username=%s, fullName=%s", targetUser.ID, targetUser.UserName, fullName)
		if err := s.AddParticipant(targetUser.ID, targetUser.UserName, fullName); err != nil {
			log.Printf("handleAddUserByUsername: failed to add participant: %v", err)
			s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при добавлении: %v", err))
			return
		}
		log.Printf("handleAddUserByUsername: successfully added participant userID=%d, username=%s", targetUser.ID, targetUser.UserName)
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Пользователь %s (@%s) добавлен в игру!", fullName, username))
		return
	}

	log.Printf("handleAddUserByUsername: user not found, checking existing participants")
	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		log.Printf("handleAddUserByUsername: failed to get participants: %v", err)
	} else {
		log.Printf("handleAddUserByUsername: checking %d existing participants", len(participants))
		for _, participant := range participants {
			if strings.EqualFold(participant.Username, username) {
				log.Printf("handleAddUserByUsername: user found in existing participants, userID=%d", participant.UserID)
				s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s уже участвует в игре.", username))
				return
			}
		}
	}

	log.Printf("handleAddUserByUsername: user @%s not found, sending error message", username)

	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		errorMsg := fmt.Sprintf("❌ Не удалось найти пользователя @%s в группе.\n\n", username)
		errorMsg += fmt.Sprintf("*Информация:*\n• Сохранено ботом: %d пользователей\n\n", len(participants))
		errorMsg += "*Важно:* Telegram Bot API не позволяет получить список всех участников группы.\n\n"
		errorMsg += fmt.Sprintf("*Как добавить участника:*\n"+
			"1. *Выберите пользователя из списка:* Начните печатать @%s и выберите пользователя из предложенного списка (не просто напечатайте @username)\n"+
			"2. Попросите пользователя @%s написать любое сообщение в группе (бот автоматически сохранит его информацию), затем повторите команду\n"+
			"3. Перешлите любое сообщение от пользователя @%s боту\n\n"+
			"*Совет:* Самый надежный способ - выбрать пользователя из списка при упоминании (начните печатать @ и выберите из списка).\n\n"+
			"Используйте /members чтобы посмотреть статистику группы.", username, username, username)
		s.sendMessage(msg.Chat.ID, errorMsg)
	} else {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Не удалось найти пользователя @%s.\n\n"+
			"*Как добавить участника по username:*\n"+
			"1. Перешлите любое сообщение от пользователя @%s боту\n"+
			"2. Или попросите пользователя @%s написать боту /add\n\n"+
			"*Альтернатива:* Если вы знаете username, перешлите сообщение от этого пользователя боту.", username, username, username))
	}
}

func (s *SecretSantaBot) HandleForwardedMessage(msg *tgbotapi.Message) {
	if msg.ForwardFrom == nil {
		return
	}

	text := strings.ToLower(msg.Text)
	caption := strings.ToLower(msg.Caption)

	hasAddUserCommand := strings.Contains(text, "/adduser") || strings.Contains(caption, "/adduser")

	if !hasAddUserCommand {
		return
	}

	var username string
	if strings.Contains(text, "/adduser") {
		parts := strings.Fields(text)
		for i, part := range parts {
			if part == "/adduser" && i+1 < len(parts) {
				username = strings.TrimPrefix(parts[i+1], "@")
				break
			}
		}
	} else if strings.Contains(caption, "/adduser") {
		parts := strings.Fields(caption)
		for i, part := range parts {
			if part == "/adduser" && i+1 < len(parts) {
				username = strings.TrimPrefix(parts[i+1], "@")
				break
			}
		}
	}

	if username == "" && msg.ForwardFrom.UserName != "" {
		username = msg.ForwardFrom.UserName
	}

	if username == "" {
		return
	}

	forwardedUsername := msg.ForwardFrom.UserName
	if forwardedUsername != "" && !strings.EqualFold(forwardedUsername, username) {
		return
	}

	existing, err := s.Storage.GetParticipant(msg.ForwardFrom.ID)
	if err == nil && existing != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s уже участвует в игре.", username))
		return
	}

	fullName := msg.ForwardFrom.FirstName
	if msg.ForwardFrom.LastName != "" {
		fullName += " " + msg.ForwardFrom.LastName
	}
	if err := s.AddParticipant(msg.ForwardFrom.ID, msg.ForwardFrom.UserName, fullName); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при добавлении: %v", err))
		return
	}
	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Пользователь %s (@%s) добавлен в игру!", fullName, username))
}

func (s *SecretSantaBot) handleRemoveParticipant(msg *tgbotapi.Message) {
	userID := msg.From.ID
	existing, err := s.Storage.GetParticipant(userID)
	if err != nil || existing == nil {
		s.sendMessage(msg.Chat.ID, "❌ Вы не участвуете в игре.")
		return
	}

	if err := s.RemoveParticipant(userID); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при удалении: %v", err))
		return
	}
	s.sendMessage(msg.Chat.ID, "✅ Вы удалены из игры.")
}

func (s *SecretSantaBot) handleListParticipants(msg *tgbotapi.Message) {
	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения списка участников: %v", err))
		return
	}

	if len(participants) == 0 {
		s.sendMessage(msg.Chat.ID, "📝 Участников пока нет.")
		return
	}

	var list strings.Builder
	list.WriteString("📝 *Участники:*\\n\\n")
	index := 1
	for _, p := range participants {
		escapedName := escapeMarkdown(p.FullName)
		list.WriteString(fmt.Sprintf("%d\\. %s", index, escapedName))
		if p.Username != "" {
			escapedUsername := escapeMarkdown(p.Username)
			list.WriteString(fmt.Sprintf(" \\(@%s\\)", escapedUsername))
		}
		list.WriteString("\\n")
		index++
	}

	response := tgbotapi.NewMessage(msg.Chat.ID, list.String())
	response.ParseMode = "MarkdownV2"
	_, err = s.Bot.Send(response)
	if err != nil {
		log.Printf("Ошибка отправки списка участников: %v", err)
		var plainList strings.Builder
		plainList.WriteString("📝 Участники:\n\n")
		index = 1
		for _, p := range participants {
			plainList.WriteString(fmt.Sprintf("%d. %s", index, p.FullName))
			if p.Username != "" {
				plainList.WriteString(fmt.Sprintf(" (@%s)", p.Username))
			}
			plainList.WriteString("\n")
			index++
		}
		responsePlain := tgbotapi.NewMessage(msg.Chat.ID, plainList.String())
		s.Bot.Send(responsePlain)
	}
}

func (s *SecretSantaBot) handleAddRestriction(msg *tgbotapi.Message) {
	userID := msg.From.ID
	existing, err := s.Storage.GetParticipant(userID)
	if err != nil || existing == nil {
		s.sendMessage(msg.Chat.ID, "❌ Сначала добавьте себя в игру через /add")
		return
	}

	text := strings.TrimSpace(msg.CommandArguments())
	if text == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите username пользователя. Пример: /restrict @username")
		return
	}

	username := strings.TrimPrefix(text, "@")

	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	var forbiddenUserID int64
	found := false
	for id, p := range participants {
		if strings.EqualFold(p.Username, username) {
			forbiddenUserID = id
			found = true
			break
		}
	}

	if !found {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s не найден среди участников.", username))
		return
	}

	if userID == forbiddenUserID {
		s.sendMessage(msg.Chat.ID, "❌ Нельзя добавить ограничение на самого себя.")
		return
	}

	creatorID := msg.From.ID

	hasRestriction, err := s.Storage.HasRestriction(userID, forbiddenUserID)
	if err != nil {
		log.Printf("handleAddRestriction: failed to check existing restriction: %v", err)
	} else if hasRestriction {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("ℹ️ Ограничение уже существует: вы не получите @%s", username))
		return
	}

	if err := s.AddRestriction(userID, forbiddenUserID, creatorID); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при добавлении ограничения: %v", err))
		return
	}
	log.Printf("handleAddRestriction: restriction saved to Redis successfully")
	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Ограничение добавлено и сохранено: вы не получите @%s", username))
}

func (s *SecretSantaBot) handleRemoveRestriction(msg *tgbotapi.Message) {
	userID := msg.From.ID
	text := strings.TrimSpace(msg.CommandArguments())
	if text == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите username пользователя. Пример: /unrestrict @username")
		return
	}

	username := msg.From.UserName
	isAdmin := s.IsAdmin(username)

	usernameArg := strings.TrimPrefix(text, "@")

	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	var forbiddenUserID int64
	found := false
	for id, p := range participants {
		if strings.EqualFold(p.Username, usernameArg) {
			forbiddenUserID = id
			found = true
			break
		}
	}

	if !found {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s не найден.", usernameArg))
		return
	}

	if !isAdmin {
		creatorID, err := s.Storage.GetRestrictionCreator(userID, forbiddenUserID)
		if err != nil || creatorID != userID {
			s.sendMessage(msg.Chat.ID, "❌ Вы можете удалить только свои ограничения. Администраторы могут удалять любые ограничения.")
			return
		}
	}

	if err := s.RemoveRestriction(userID, forbiddenUserID); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при удалении ограничения: %v", err))
		return
	}
	log.Printf("handleRemoveRestriction: restriction deleted from Redis successfully")
	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Ограничение удалено из Redis для @%s", usernameArg))
}

func (s *SecretSantaBot) handleListRestrictions(msg *tgbotapi.Message) {
	userID := msg.From.ID
	username := msg.From.UserName
	isAdmin := s.IsAdmin(username)

	restrictions, _, err := s.Storage.GetAllRestrictions()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения ограничений: %v", err))
		return
	}

	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	var list strings.Builder
	list.WriteString("📋 *Ограничения:*\\n\\n")

	hasRestrictions := false

	if isAdmin {
		for userID, userRestrictions := range restrictions {
			if len(userRestrictions) == 0 {
				continue
			}
			user := participants[userID]
			if user == nil {
				continue
			}

			escapedUserName := escapeMarkdown(user.FullName)
			list.WriteString(fmt.Sprintf("*%s* не получит:\\n", escapedUserName))
			for forbiddenID := range userRestrictions {
				forbiddenUser := participants[forbiddenID]
				if forbiddenUser != nil {
					escapedForbiddenName := escapeMarkdown(forbiddenUser.FullName)
					list.WriteString(fmt.Sprintf("  \\- %s", escapedForbiddenName))
					if forbiddenUser.Username != "" {
						escapedForbiddenUsername := escapeMarkdown(forbiddenUser.Username)
						list.WriteString(fmt.Sprintf(" \\(@%s\\)", escapedForbiddenUsername))
					}
					list.WriteString("\\n")
				}
			}
			list.WriteString("\\n")
			hasRestrictions = true
		}
	} else {
		userRestrictions, exists := restrictions[userID]
		if !exists || len(userRestrictions) == 0 {
			s.sendMessage(msg.Chat.ID, "📋 У вас нет ограничений.")
			return
		}

		user := participants[userID]
		if user == nil {
			s.sendMessage(msg.Chat.ID, "❌ Ошибка: вы не найдены среди участников.")
			return
		}

		list.WriteString("*Вы* не получите:\\n")
		for forbiddenID := range userRestrictions {
			forbiddenUser := participants[forbiddenID]
			if forbiddenUser != nil {
				escapedForbiddenName := escapeMarkdown(forbiddenUser.FullName)
				list.WriteString(fmt.Sprintf("  \\- %s", escapedForbiddenName))
				if forbiddenUser.Username != "" {
					escapedForbiddenUsername := escapeMarkdown(forbiddenUser.Username)
					list.WriteString(fmt.Sprintf(" \\(@%s\\)", escapedForbiddenUsername))
				}
				list.WriteString("\\n")
			}
		}
		hasRestrictions = true
	}

	if !hasRestrictions {
		s.sendMessage(msg.Chat.ID, "📋 Ограничений нет.")
		return
	}

	response := tgbotapi.NewMessage(msg.Chat.ID, list.String())
	response.ParseMode = "MarkdownV2"
	_, err = s.Bot.Send(response)
	if err != nil {
		log.Printf("Failed to send restrictions list: %v", err)
		var plainList strings.Builder
		plainList.WriteString("📋 Ограничения:\n\n")
		if isAdmin {
			for userID, userRestrictions := range restrictions {
				if len(userRestrictions) == 0 {
					continue
				}
				user := participants[userID]
				if user == nil {
					continue
				}
				plainList.WriteString(fmt.Sprintf("%s не получит:\n", user.FullName))
				for forbiddenID := range userRestrictions {
					forbiddenUser := participants[forbiddenID]
					if forbiddenUser != nil {
						plainList.WriteString(fmt.Sprintf("  - %s", forbiddenUser.FullName))
						if forbiddenUser.Username != "" {
							plainList.WriteString(fmt.Sprintf(" (@%s)", forbiddenUser.Username))
						}
						plainList.WriteString("\n")
					}
				}
				plainList.WriteString("\n")
			}
		} else {
			userRestrictions := restrictions[userID]
			plainList.WriteString("Вы не получите:\n")
			for forbiddenID := range userRestrictions {
				forbiddenUser := participants[forbiddenID]
				if forbiddenUser != nil {
					plainList.WriteString(fmt.Sprintf("  - %s", forbiddenUser.FullName))
					if forbiddenUser.Username != "" {
						plainList.WriteString(fmt.Sprintf(" (@%s)", forbiddenUser.Username))
					}
					plainList.WriteString("\n")
				}
			}
		}
		responsePlain := tgbotapi.NewMessage(msg.Chat.ID, plainList.String())
		s.Bot.Send(responsePlain)
	}
}

func (s *SecretSantaBot) handleGenerate(msg *tgbotapi.Message) {
	username := msg.From.UserName
	if !s.IsAdmin(username) {
		s.sendMessage(msg.Chat.ID, "❌ Эта команда доступна только администраторам.")
		return
	}

	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	if len(participants) < 2 {
		s.sendMessage(msg.Chat.ID, "❌ Нужно минимум 2 участника для игры.")
		return
	}

	err = s.GenerateAssignments()
	if err != nil {
		escapedError := escapeMarkdown(err.Error())
		errorMsg := fmt.Sprintf("❌ *Ошибка при генерации распределения:*\\n\\n%s\\n\\n"+
			"*Возможные причины:*\\n"+
			"• Слишком много ограничений\\n"+
			"• Невозможно создать валидное распределение с текущими ограничениями\\n\\n"+
			"*Решение:*\\n"+
			"Попробуйте уменьшить количество ограничений или изменить их\\.", escapedError)
		s.sendMessage(msg.Chat.ID, errorMsg)
		if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
			adminUserID := msg.From.ID
			adminMsg := tgbotapi.NewMessage(adminUserID, errorMsg)
			adminMsg.ParseMode = "MarkdownV2"
			s.Bot.Send(adminMsg)
		}
		return
	}

	if err := s.Storage.SaveGameState(true, false); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка сохранения состояния игры: %v", err))
		return
	}
	s.sendMessage(msg.Chat.ID, "✅ Распределение успешно создано! Используйте /startgame чтобы начать игру и отправить результаты участникам.")
}

func (s *SecretSantaBot) handleSendAssignments(msg *tgbotapi.Message) {
	username := msg.From.UserName
	if !s.IsAdmin(username) {
		s.sendMessage(msg.Chat.ID, "❌ Эта команда доступна только администраторам.")
		return
	}

	gameActive, _, err := s.Storage.GetGameState()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения состояния игры: %v", err))
		return
	}

	if !gameActive {
		s.sendMessage(msg.Chat.ID, "❌ Сначала создайте распределение через /generate")
		return
	}

	assignments, err := s.Storage.GetAllAssignments()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения назначений: %v", err))
		return
	}

	if len(assignments) == 0 {
		s.sendMessage(msg.Chat.ID, "❌ Сначала создайте распределение через /generate")
		return
	}

	successCount := 0
	failedCount := 0

	for userID := range assignments {
		err := s.SendAssignment(userID)
		if err != nil {
			log.Printf("Failed to send message to user %d: %v", userID, err)
			failedCount++
		} else {
			successCount++
		}
	}

	resultMsg := fmt.Sprintf("✅ *Игра начата!*\n\n"+
		"Отправлено сообщений: %d\n"+
		"Ошибок: %d\n\n"+
		"Все участники получили информацию о своих получателях.", successCount, failedCount)
	s.sendMessage(msg.Chat.ID, resultMsg)

	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		adminUserID := msg.From.ID
		adminMsg := tgbotapi.NewMessage(adminUserID, resultMsg)
		adminMsg.ParseMode = "Markdown"
		s.Bot.Send(adminMsg)
	}

	if err := s.Storage.SaveGameState(true, true); err != nil {
		log.Printf("Failed to save game state: %v", err)
	}
}

func (s *SecretSantaBot) handleReset(msg *tgbotapi.Message) {
	if err := s.Storage.ClearAll(); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при сбросе игры: %v", err))
		return
	}
	s.sendMessage(msg.Chat.ID, "🔄 Игра сброшена. Можно начинать заново!")
}

func (s *SecretSantaBot) handleStatus(msg *tgbotapi.Message) {
	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения статуса: %v", err))
		return
	}

	gameActive, gameStarted, err := s.Storage.GetGameState()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения состояния игры: %v", err))
		return
	}

	gameActiveText := map[bool]string{true: "✅ Да", false: "❌ Нет"}[gameActive]
	gameStartedText := map[bool]string{true: "✅ Да", false: "❌ Нет"}[gameStarted]

	status := fmt.Sprintf("📊 *Статус игры:*\\n\\n"+
		"Участников: %d\\n"+
		"Распределение создано: %s\\n"+
		"Результаты отправлены: %s",
		len(participants),
		escapeMarkdown(gameActiveText),
		escapeMarkdown(gameStartedText))

	response := tgbotapi.NewMessage(msg.Chat.ID, status)
	response.ParseMode = "MarkdownV2"
	_, err = s.Bot.Send(response)
	if err != nil {
		log.Printf("Failed to send status: %v", err)
		statusPlain := fmt.Sprintf("📊 Статус игры:\n\n"+
			"Участников: %d\n"+
			"Распределение создано: %s\n"+
			"Результаты отправлены: %s",
			len(participants), gameActiveText, gameStartedText)
		responsePlain := tgbotapi.NewMessage(msg.Chat.ID, statusPlain)
		s.Bot.Send(responsePlain)
	}
}

func (s *SecretSantaBot) handleMembersCount(msg *tgbotapi.Message) {
	if !msg.Chat.IsGroup() && !msg.Chat.IsSuperGroup() {
		s.sendMessage(msg.Chat.ID, "❌ Эта команда работает только в группах.")
		return
	}

	participants, err := s.Storage.GetAllParticipants()
	savedCount := 0
	gameParticipants := 0
	if err == nil {
		savedCount = len(participants)
		for _, p := range participants {
			if p.Username != "" {
				gameParticipants++
			}
		}
	}

	admins, err := s.Bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: msg.Chat.ID},
	})
	adminCount := 0
	if err == nil {
		adminCount = len(admins)
	}

	var membersCount int
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMemberCount?chat_id=%d", s.Bot.Token, msg.Chat.ID)
	resp, err := http.Get(apiURL)
	if err == nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var result struct {
				OK     bool `json:"ok"`
				Result int  `json:"result"`
			}
			if json.Unmarshal(body, &result) == nil && result.OK {
				membersCount = result.Result
				log.Printf("handleMembersCount: group has %d members", membersCount)
			}
		}
	} else {
		log.Printf("handleMembersCount: failed to get member count: %v", err)
	}

	message := "📊 *Информация о группе:*\n\n"
	if membersCount > 0 {
		message += fmt.Sprintf("Участников в группе: %d\n", membersCount)
	}
	message += fmt.Sprintf("Администраторов: %d\n", adminCount)
	message += fmt.Sprintf("Сохранено ботом: %d пользователей\n", savedCount)
	message += fmt.Sprintf("Участвует в игре: %d", gameParticipants)

	s.sendMessage(msg.Chat.ID, message)
}

func (s *SecretSantaBot) CheckTriggerWords(msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}

	text := strings.ToLower(msg.Text)

	allTriggerWords, err := s.Storage.GetAllTriggerWords()
	if err == nil {
		for _, triggerWord := range allTriggerWords {
			if strings.Contains(text, strings.ToLower(triggerWord)) {
				messages, err := s.Storage.GetTriggerMessages(triggerWord)
				if err == nil && len(messages) > 0 {
					randomMessage := messages[rand.Intn(len(messages))]
					s.sendMessage(msg.Chat.ID, randomMessage)
					log.Printf("Trigger word '%s' detected in message from user %d, sent random message (total: %d)", triggerWord, msg.From.ID, len(messages))
					return
				}
			}
		}
	}

	for _, triggerWord := range s.TriggerWords {
		if strings.Contains(text, strings.ToLower(triggerWord)) {
			messages, err := s.Storage.GetTriggerMessages(triggerWord)
			if err == nil && len(messages) > 0 {
				randomMessage := messages[rand.Intn(len(messages))]
				s.sendMessage(msg.Chat.ID, randomMessage)
				log.Printf("Config trigger word '%s' detected in message from user %d, sent random message (total: %d)", triggerWord, msg.From.ID, len(messages))
			} else {
				curseMessage := "💩 Санта проклинает тебя на понос и желает дерьмового нового года! 💩"
				s.sendMessage(msg.Chat.ID, curseMessage)
				log.Printf("Config trigger word '%s' detected in message from user %d, sent default message (no custom messages found)", triggerWord, msg.From.ID)
			}
			return
		}
	}

	userTriggers := s.UserTriggers[msg.From.ID]
	for _, triggerWord := range userTriggers {
		if strings.Contains(text, strings.ToLower(triggerWord)) {
			messages, err := s.Storage.GetTriggerMessages(triggerWord)
			if err == nil && len(messages) > 0 {
				randomMessage := messages[rand.Intn(len(messages))]
				s.sendMessage(msg.Chat.ID, randomMessage)
				log.Printf("User trigger word '%s' detected in message from user %d, sent random message (total: %d)", triggerWord, msg.From.ID, len(messages))
			} else {
				curseMessage := "💩 Санта проклинает тебя на понос и желает дерьмового нового года! 💩"
				s.sendMessage(msg.Chat.ID, curseMessage)
				log.Printf("User trigger word '%s' detected in message from user %d, sent default message (no custom messages found)", triggerWord, msg.From.ID)
			}
			return
		}
	}
}

func (s *SecretSantaBot) handleSetWish(msg *tgbotapi.Message) {
	userID := msg.From.ID
	existing, err := s.Storage.GetParticipant(userID)
	if err != nil || existing == nil {
		s.sendMessage(msg.Chat.ID, "❌ Сначала добавьте себя в игру через /add")
		return
	}

	wish := strings.TrimSpace(msg.CommandArguments())
	if wish == "" {
		currentWish, err := s.Storage.GetWish(userID)
		if err == nil && currentWish != "" {
			s.sendMessage(msg.Chat.ID, fmt.Sprintf("💝 Ваше текущее желание:\n\n%s\n\nЧтобы изменить, используйте: /wish новое желание", currentWish))
		} else {
			s.sendMessage(msg.Chat.ID, "❌ Укажите ваше желание. Пример: /wish Хочу получить книгу")
		}
		return
	}

	if err := s.Storage.SaveWish(userID, wish); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при сохранении желания: %v", err))
		return
	}

	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Ваше желание сохранено:\n\n%s\n\nВы можете изменить его в любой момент, используя /wish новое желание", wish))
}

func (s *SecretSantaBot) handleGetWish(msg *tgbotapi.Message) {
	userID := msg.From.ID
	existing, err := s.Storage.GetParticipant(userID)
	if err != nil || existing == nil {
		s.sendMessage(msg.Chat.ID, "❌ Сначала добавьте себя в игру через /add")
		return
	}

	wish, err := s.Storage.GetWish(userID)
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при получении желания: %v", err))
		return
	}

	if wish == "" {
		s.sendMessage(msg.Chat.ID, "💝 У вас пока нет сохраненного желания.\n\nИспользуйте /wish ваше желание чтобы добавить его.")
	} else {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("💝 Ваше желание:\n\n%s", wish))
	}
}

func (s *SecretSantaBot) handleDeleteWish(msg *tgbotapi.Message) {
	userID := msg.From.ID
	existing, err := s.Storage.GetParticipant(userID)
	if err != nil || existing == nil {
		s.sendMessage(msg.Chat.ID, "❌ Сначала добавьте себя в игру через /add")
		return
	}

	if err := s.Storage.DeleteWish(userID); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при удалении желания: %v", err))
		return
	}

	s.sendMessage(msg.Chat.ID, "✅ Ваше желание удалено.")
}

func (s *SecretSantaBot) handleAddTrigger(msg *tgbotapi.Message) {
	userID := msg.From.ID
	triggerWord := strings.TrimSpace(msg.CommandArguments())
	if triggerWord == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите слово-триггер. Пример: /addtrigger плохое_слово")
		return
	}

	triggerWord = strings.ToLower(triggerWord)

	if s.UserTriggers[userID] == nil {
		s.UserTriggers[userID] = make([]string, 0)
	}

	for _, existing := range s.UserTriggers[userID] {
		if existing == triggerWord {
			s.sendMessage(msg.Chat.ID, fmt.Sprintf("ℹ️ Слово '%s' уже добавлено в ваши триггеры", triggerWord))
			return
		}
	}

	s.UserTriggers[userID] = append(s.UserTriggers[userID], triggerWord)
	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Слово-триггер '%s' добавлено! Теперь при упоминании этого слова бот отправит специальное сообщение.", triggerWord))
	log.Printf("User %d added trigger word: %s", userID, triggerWord)
}

func (s *SecretSantaBot) handleAddTriggerMessage(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите триггерное слово и сообщение. Пример: /addtriggermessage слово|Сообщение для отправки")
		return
	}

	parts := strings.SplitN(args, "|", 2)
	if len(parts) != 2 {
		s.sendMessage(msg.Chat.ID, "❌ Неверный формат. Используйте: /addtriggermessage слово|Сообщение для отправки\n\nПример: /addtriggermessage мат|💩 Санта проклинает тебя!")
		return
	}

	triggerWord := strings.ToLower(strings.TrimSpace(parts[0]))
	message := strings.TrimSpace(parts[1])

	if triggerWord == "" || message == "" {
		s.sendMessage(msg.Chat.ID, "❌ Триггерное слово и сообщение не могут быть пустыми.")
		return
	}

	if err := s.Storage.SaveTriggerMessage(triggerWord, message); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при сохранении сообщения: %v", err))
		return
	}

	messages, err := s.Storage.GetTriggerMessages(triggerWord)
	if err == nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Сообщение добавлено к триггеру '%s'!\n\nВсего сообщений для этого триггера: %d\n\nПри обнаружении слова '%s' бот случайным образом выберет одно из сообщений.", triggerWord, len(messages), triggerWord))
	} else {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Сообщение добавлено к триггеру '%s'!", triggerWord))
	}
	log.Printf("User %d added trigger message for word '%s': %s", msg.From.ID, triggerWord, message)
}

func (s *SecretSantaBot) handleAddComment(msg *tgbotapi.Message) {
	userID := msg.From.ID
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		s.sendMessage(msg.Chat.ID, "❌ Укажите участника и комментарий. Пример: /comment @username Текст комментария")
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		s.sendMessage(msg.Chat.ID, "❌ Неверный формат. Используйте: /comment @username Текст комментария")
		return
	}

	usernameArg := strings.TrimPrefix(parts[0], "@")
	commentText := strings.Join(parts[1:], " ")

	participants, err := s.Storage.GetAllParticipants()
	if err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	var receiverID int64
	found := false
	for id, p := range participants {
		if strings.EqualFold(p.Username, usernameArg) {
			receiverID = id
			found = true
			break
		}
	}

	if !found {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Пользователь @%s не найден среди участников.", usernameArg))
		return
	}

	if userID == receiverID {
		s.sendMessage(msg.Chat.ID, "❌ Нельзя добавить комментарий для самого себя.")
		return
	}

	if err := s.Storage.SaveComment(receiverID, userID, commentText); err != nil {
		s.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка при сохранении комментария: %v", err))
		return
	}

	receiver := participants[receiverID]
	receiverName := receiver.FullName
	if receiver.Username != "" {
		receiverName += " (@" + receiver.Username + ")"
	}

	s.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Комментарий добавлен для %s!\n\n💬 Ваш комментарий:\n%s", receiverName, commentText))
	log.Printf("User %d added comment for receiverID=%d: %s", userID, receiverID, commentText)
}

func (s *SecretSantaBot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	s.Bot.Send(msg)
}
