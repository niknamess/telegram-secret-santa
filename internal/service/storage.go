package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"telegram-secret-santa/internal/domain"

	"github.com/redis/go-redis/v9"
)

type Storage struct {
	client *redis.Client
	ctx    context.Context
}

func NewStorage(host, port, password string, db int) (*Storage, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
	})

	ctx := context.Background()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Storage{
		client: rdb,
		ctx:    ctx,
	}, nil
}

func participantKey(userID int64) string {
	return fmt.Sprintf("participant:%d", userID)
}

func restrictionKey(userID, forbiddenUserID int64) string {
	return fmt.Sprintf("restriction:%d:%d", userID, forbiddenUserID)
}

func restrictionCreatorKey(userID, forbiddenUserID int64) string {
	return fmt.Sprintf("restriction_creator:%d:%d", userID, forbiddenUserID)
}

func assignmentKey(giverID int64) string {
	return fmt.Sprintf("assignment:%d", giverID)
}

func gameStateKey() string {
	return "game:state"
}

func (s *Storage) SaveParticipant(p *domain.Participant) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to serialize participant: %w", err)
	}

	key := participantKey(p.UserID)
	return s.client.Set(s.ctx, key, data, 0).Err()
}

func (s *Storage) GetParticipant(userID int64) (*domain.Participant, error) {
	key := participantKey(userID)
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get participant: %w", err)
	}

	var p domain.Participant
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, fmt.Errorf("failed to deserialize participant: %w", err)
	}

	return &p, nil
}

func (s *Storage) GetAllParticipants() (map[int64]*domain.Participant, error) {
	participants := make(map[int64]*domain.Participant)

	keys, err := s.client.Keys(s.ctx, "participant:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get participant keys: %w", err)
	}

	for _, key := range keys {
		data, err := s.client.Get(s.ctx, key).Result()
		if err != nil {
			continue
		}

		var p domain.Participant
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			continue
		}

		participants[p.UserID] = &p
	}

	return participants, nil
}

func (s *Storage) DeleteParticipant(userID int64) error {
	key := participantKey(userID)
	return s.client.Del(s.ctx, key).Err()
}

func (s *Storage) SaveRestriction(userID, forbiddenUserID, creatorID int64) error {
	key := restrictionKey(userID, forbiddenUserID)
	creatorKey := restrictionCreatorKey(userID, forbiddenUserID)

	if err := s.client.Set(s.ctx, key, "1", 0).Err(); err != nil {
		return fmt.Errorf("failed to save restriction: %w", err)
	}

	if err := s.client.Set(s.ctx, creatorKey, strconv.FormatInt(creatorID, 10), 0).Err(); err != nil {
		return fmt.Errorf("failed to save restriction creator: %w", err)
	}

	return nil
}

func (s *Storage) HasRestriction(userID, forbiddenUserID int64) (bool, error) {
	key := restrictionKey(userID, forbiddenUserID)
	exists, err := s.client.Exists(s.ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check restriction: %w", err)
	}
	return exists > 0, nil
}

func (s *Storage) GetRestrictionCreator(userID, forbiddenUserID int64) (int64, error) {
	key := restrictionCreatorKey(userID, forbiddenUserID)
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get restriction creator: %w", err)
	}

	creatorID, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse creator ID: %w", err)
	}

	return creatorID, nil
}

func (s *Storage) GetAllRestrictions() (map[int64]map[int64]bool, map[int64]map[int64]int64, error) {
	restrictions := make(map[int64]map[int64]bool)
	creators := make(map[int64]map[int64]int64)

	keys, err := s.client.Keys(s.ctx, "restriction:*").Result()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get restriction keys: %w", err)
	}

	for _, key := range keys {
		var userID, forbiddenUserID int64
		_, err := fmt.Sscanf(key, "restriction:%d:%d", &userID, &forbiddenUserID)
		if err != nil {
			continue
		}

		if restrictions[userID] == nil {
			restrictions[userID] = make(map[int64]bool)
		}
		restrictions[userID][forbiddenUserID] = true

		creatorID, err := s.GetRestrictionCreator(userID, forbiddenUserID)
		if err == nil && creatorID != 0 {
			if creators[userID] == nil {
				creators[userID] = make(map[int64]int64)
			}
			creators[userID][forbiddenUserID] = creatorID
		}
	}

	return restrictions, creators, nil
}

func (s *Storage) DeleteRestriction(userID, forbiddenUserID int64) error {
	key := restrictionKey(userID, forbiddenUserID)
	creatorKey := restrictionCreatorKey(userID, forbiddenUserID)

	if err := s.client.Del(s.ctx, key, creatorKey).Err(); err != nil {
		return fmt.Errorf("failed to delete restriction: %w", err)
	}

	return nil
}

func (s *Storage) DeleteAllRestrictionsForUser(userID int64) error {
	pattern := fmt.Sprintf("restriction:%d:*", userID)
	keys, err := s.client.Keys(s.ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get restriction keys: %w", err)
	}

	if len(keys) > 0 {
		creatorKeys := make([]string, 0, len(keys))
		for _, key := range keys {
			var forbiddenUserID int64
			_, err := fmt.Sscanf(key, "restriction:%d:%d", &userID, &forbiddenUserID)
			if err == nil {
				creatorKeys = append(creatorKeys, restrictionCreatorKey(userID, forbiddenUserID))
			}
		}
		keys = append(keys, creatorKeys...)
		return s.client.Del(s.ctx, keys...).Err()
	}

	return nil
}

func (s *Storage) SaveAssignment(giverID, receiverID int64) error {
	key := assignmentKey(giverID)
	return s.client.Set(s.ctx, key, strconv.FormatInt(receiverID, 10), 0).Err()
}

func (s *Storage) GetAssignment(giverID int64) (int64, error) {
	key := assignmentKey(giverID)
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get assignment: %w", err)
	}

	receiverID, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse receiver ID: %w", err)
	}

	return receiverID, nil
}

func (s *Storage) GetAllAssignments() (map[int64]int64, error) {
	assignments := make(map[int64]int64)

	keys, err := s.client.Keys(s.ctx, "assignment:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment keys: %w", err)
	}

	for _, key := range keys {
		var giverID int64
		_, err := fmt.Sscanf(key, "assignment:%d", &giverID)
		if err != nil {
			continue
		}

		receiverID, err := s.GetAssignment(giverID)
		if err == nil {
			assignments[giverID] = receiverID
		}
	}

	return assignments, nil
}

func (s *Storage) DeleteAssignment(giverID int64) error {
	key := assignmentKey(giverID)
	return s.client.Del(s.ctx, key).Err()
}

func (s *Storage) DeleteAllAssignments() error {
	keys, err := s.client.Keys(s.ctx, "assignment:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get assignment keys: %w", err)
	}

	if len(keys) > 0 {
		return s.client.Del(s.ctx, keys...).Err()
	}

	return nil
}

func (s *Storage) SaveGameState(gameActive, gameStarted bool) error {
	key := gameStateKey()
	data := fmt.Sprintf("%t:%t", gameActive, gameStarted)
	return s.client.Set(s.ctx, key, data, 0).Err()
}

func (s *Storage) GetGameState() (bool, bool, error) {
	key := gameStateKey()
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("failed to get game state: %w", err)
	}

	var gameActive, gameStarted bool
	_, err = fmt.Sscanf(data, "%t:%t", &gameActive, &gameStarted)
	if err != nil {
		return false, false, fmt.Errorf("failed to parse game state: %w", err)
	}

	return gameActive, gameStarted, nil
}

func (s *Storage) ResetGameState() error {
	key := gameStateKey()
	return s.client.Del(s.ctx, key).Err()
}

func (s *Storage) ClearAll() error {
	keys, err := s.client.Keys(s.ctx, "*").Result()
	if err != nil {
		return fmt.Errorf("failed to get all keys: %w", err)
	}

	if len(keys) > 0 {
		return s.client.Del(s.ctx, keys...).Err()
	}

	return nil
}

func wishKey(userID int64) string {
	return fmt.Sprintf("wish:%d", userID)
}

func (s *Storage) SaveWish(userID int64, wish string) error {
	key := wishKey(userID)
	return s.client.Set(s.ctx, key, wish, 0).Err()
}

func (s *Storage) GetWish(userID int64) (string, error) {
	key := wishKey(userID)
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get wish: %w", err)
	}
	return data, nil
}

func (s *Storage) DeleteWish(userID int64) error {
	key := wishKey(userID)
	return s.client.Del(s.ctx, key).Err()
}

func triggerMessagesKey(triggerWord string) string {
	return fmt.Sprintf("trigger_messages:%s", triggerWord)
}

func (s *Storage) SaveTriggerMessage(triggerWord, message string) error {
	key := triggerMessagesKey(triggerWord)
	existing, err := s.GetTriggerMessages(triggerWord)
	if err != nil {
		return fmt.Errorf("failed to get existing messages: %w", err)
	}

	for _, msg := range existing {
		if msg == message {
			return nil
		}
	}

	existing = append(existing, message)
	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to serialize messages: %w", err)
	}

	return s.client.Set(s.ctx, key, data, 0).Err()
}

func (s *Storage) GetTriggerMessages(triggerWord string) ([]string, error) {
	key := triggerMessagesKey(triggerWord)
	data, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trigger messages: %w", err)
	}

	var messages []string
	if err := json.Unmarshal([]byte(data), &messages); err != nil {
		return nil, fmt.Errorf("failed to deserialize messages: %w", err)
	}

	return messages, nil
}

func (s *Storage) GetAllTriggerWords() ([]string, error) {
	keys, err := s.client.Keys(s.ctx, "trigger_messages:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get trigger message keys: %w", err)
	}

	triggerWords := make([]string, 0, len(keys))
	for _, key := range keys {
		triggerWord := strings.TrimPrefix(key, "trigger_messages:")
		if triggerWord != "" {
			triggerWords = append(triggerWords, triggerWord)
		}
	}

	return triggerWords, nil
}

func (s *Storage) DeleteTriggerMessage(triggerWord, message string) error {
	key := triggerMessagesKey(triggerWord)
	existing, err := s.GetTriggerMessages(triggerWord)
	if err != nil {
		return fmt.Errorf("failed to get existing messages: %w", err)
	}

	var updated []string
	for _, msg := range existing {
		if msg != message {
			updated = append(updated, msg)
		}
	}

	if len(updated) == 0 {
		return s.client.Del(s.ctx, key).Err()
	}

	data, err := json.Marshal(updated)
	if err != nil {
		return fmt.Errorf("failed to serialize messages: %w", err)
	}

	return s.client.Set(s.ctx, key, data, 0).Err()
}

func commentKey(receiverID, authorID int64) string {
	return fmt.Sprintf("comment:%d:%d", receiverID, authorID)
}

func commentKeysPattern(receiverID int64) string {
	return fmt.Sprintf("comment:%d:*", receiverID)
}

func (s *Storage) SaveComment(receiverID, authorID int64, comment string) error {
	key := commentKey(receiverID, authorID)
	return s.client.Set(s.ctx, key, comment, 0).Err()
}

func (s *Storage) GetComments(receiverID int64) (map[int64]string, error) {
	pattern := commentKeysPattern(receiverID)
	keys, err := s.client.Keys(s.ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get comment keys: %w", err)
	}

	comments := make(map[int64]string)
	for _, key := range keys {
		var keyReceiverID, authorID int64
		_, err := fmt.Sscanf(key, "comment:%d:%d", &keyReceiverID, &authorID)
		if err != nil {
			continue
		}

		if keyReceiverID != receiverID {
			continue
		}

		data, err := s.client.Get(s.ctx, key).Result()
		if err == nil {
			comments[authorID] = data
		}
	}

	return comments, nil
}

func (s *Storage) DeleteComment(receiverID, authorID int64) error {
	key := commentKey(receiverID, authorID)
	return s.client.Del(s.ctx, key).Err()
}

func (s *Storage) Close() error {
	return s.client.Close()
}
