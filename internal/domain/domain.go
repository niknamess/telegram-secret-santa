package domain

type Participant struct {
	UserID   int64
	Username string
	FullName string
}

type StorageInterface interface {
	SaveParticipant(p *Participant) error
	GetParticipant(userID int64) (*Participant, error)
	GetAllParticipants() (map[int64]*Participant, error)
	DeleteParticipant(userID int64) error
	SaveRestriction(userID, forbiddenUserID, creatorID int64) error
	HasRestriction(userID, forbiddenUserID int64) (bool, error)
	GetRestrictionCreator(userID, forbiddenUserID int64) (int64, error)
	GetAllRestrictions() (map[int64]map[int64]bool, map[int64]map[int64]int64, error)
	DeleteRestriction(userID, forbiddenUserID int64) error
	DeleteAllRestrictionsForUser(userID int64) error
	SaveAssignment(giverID, receiverID int64) error
	GetAssignment(giverID int64) (int64, error)
	GetAllAssignments() (map[int64]int64, error)
	DeleteAssignment(giverID int64) error
	DeleteAllAssignments() error
	SaveGameState(gameActive, gameStarted bool) error
	GetGameState() (bool, bool, error)
	ResetGameState() error
	SaveWish(userID int64, wish string) error
	GetWish(userID int64) (string, error)
	DeleteWish(userID int64) error
	SaveTriggerMessage(triggerWord, message string) error
	GetTriggerMessages(triggerWord string) ([]string, error)
	GetAllTriggerWords() ([]string, error)
	DeleteTriggerMessage(triggerWord, message string) error
	SaveComment(receiverID, authorID int64, comment string) error
	GetComments(receiverID int64) (map[int64]string, error)
	DeleteComment(receiverID, authorID int64) error
	ClearAll() error
	Close() error
}
