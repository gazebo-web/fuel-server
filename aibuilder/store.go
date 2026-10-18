package aibuilder

import (
	"errors"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Chat and LLM keys in fuel-server's MySQL (design §4.3, §4.4). Every
// table is the agent's own: fuel-server neither reads nor migrates them.
//
// A project has one chat session. Messages are what the model sees again
// on the next turn (user text, assistant text, tool results); events are
// what the browser is shown while a turn runs, numbered so a stream can
// resume after a disconnect. A turn runs in the background, so it outlives
// the request that started it.

// ChatSession is a project's conversation, keyed by the project's UUID.
type ChatSession struct {
	ID          string `gorm:"type:varchar(36);primary_key"`
	ProjectUUID string `gorm:"type:varchar(36);unique_index"`
	Owner       string `gorm:"type:varchar(190);index"`
	CreatedAt   time.Time
}

// TableName keeps the agent's tables together.
func (ChatSession) TableName() string { return "ai_chat_sessions" }

// ChatMessage is one message of the conversation's history.
type ChatMessage struct {
	ID        uint      `gorm:"primary_key" json:"-"`
	SessionID string    `gorm:"type:varchar(36);index" json:"session_id"`
	Role      string    `gorm:"type:varchar(16)" json:"role"` // user, assistant or tool_result
	Content   string    `gorm:"type:mediumtext" json:"content"`
	ToolName  string    `gorm:"type:varchar(128)" json:"tool_name,omitempty"`
	ToolID    string    `gorm:"type:varchar(128)" json:"tool_id,omitempty"`
	CreatedAt time.Time `gorm:"type:datetime(6)" json:"timestamp"`
}

// TableName keeps the agent's tables together.
func (ChatMessage) TableName() string { return "ai_chat_messages" }

// Turn statuses.
const (
	TurnRunning   = "running"
	TurnDone      = "done"
	TurnFailed    = "failed"
	TurnCancelled = "cancelled"
)

// ChatTurn is one user message and the agent's work on it.
type ChatTurn struct {
	ID        string `gorm:"type:varchar(36);primary_key" json:"turn_id"`
	SessionID string `gorm:"type:varchar(36);index" json:"-"`
	Status    string `gorm:"type:varchar(16);index" json:"status"`
	Provider  string `gorm:"type:varchar(32)" json:"provider"`
	Model     string `gorm:"type:varchar(128)" json:"model,omitempty"`
	// Message is what the user asked.
	Message    string     `gorm:"type:text" json:"message"`
	Error      string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt  time.Time  `gorm:"type:datetime(6)" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// TableName keeps the agent's tables together.
func (ChatTurn) TableName() string { return "ai_chat_turns" }

// ChatEvent is one streamed event of a turn. ID is the SSE event id.
type ChatEvent struct {
	ID        uint   `gorm:"primary_key"`
	TurnID    string `gorm:"type:varchar(36);index"`
	Type      string `gorm:"type:varchar(32)"`
	Payload   string `gorm:"type:mediumtext"`
	CreatedAt time.Time
}

// TableName keeps the agent's tables together.
func (ChatEvent) TableName() string { return "ai_chat_events" }

// LLMKey is a user's API key for one provider, encrypted (crypto.go).
type LLMKey struct {
	ID        uint      `gorm:"primary_key" json:"-"`
	Owner     string    `gorm:"type:varchar(190);unique_index:idx_ai_llm_key" json:"user_id"`
	Provider  string    `gorm:"type:varchar(32);unique_index:idx_ai_llm_key" json:"provider"`
	Encrypted string    `gorm:"type:text" json:"-"`
	Masked    string    `gorm:"type:varchar(16)" json:"api_key_masked,omitempty"`
	Endpoint  string    `gorm:"type:varchar(512)" json:"endpoint,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName keeps the agent's tables together.
func (LLMKey) TableName() string { return "ai_llm_keys" }

// ErrNotFound is a missing row.
var ErrNotFound = errors.New("not found")

// Store reads and writes the agent's tables.
type Store struct{ db *gorm.DB }

// NewStore migrates the agent's tables and returns a store over db.
func NewStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(&ChatSession{}, &ChatMessage{}, &ChatTurn{}, &ChatEvent{}, &LLMKey{}).Error; err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Session returns the project's chat session, creating it.
func (s *Store) Session(projectUUID, owner string) (*ChatSession, error) {
	var cs ChatSession
	err := s.db.Where("project_uuid = ?", projectUUID).First(&cs).Error
	if gorm.IsRecordNotFoundError(err) {
		cs = ChatSession{ID: newID(), ProjectUUID: projectUUID, Owner: owner, CreatedAt: time.Now().UTC()}
		if err = s.db.Create(&cs).Error; err != nil && isDuplicate(err) {
			// Another request created it first.
			err = s.db.Where("project_uuid = ?", projectUUID).First(&cs).Error
		}
	}
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// Messages returns a session's history, oldest first.
func (s *Store) Messages(sessionID string) ([]ChatMessage, error) {
	var out []ChatMessage
	err := s.db.Where("session_id = ?", sessionID).Order("id").Find(&out).Error
	return out, err
}

// AppendMessage adds a message to a session's history.
func (s *Store) AppendMessage(m *ChatMessage) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return s.db.Create(m).Error
}

// ClearSession deletes a session's history, turns and events.
func (s *Store) ClearSession(sessionID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("turn_id IN (?)", tx.Model(&ChatTurn{}).Select("id").
			Where("session_id = ?", sessionID).QueryExpr()).Delete(&ChatEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", sessionID).Delete(&ChatTurn{}).Error; err != nil {
			return err
		}
		return tx.Where("session_id = ?", sessionID).Delete(&ChatMessage{}).Error
	})
}

// StartTurn records a running turn, unless the session already has one.
func (s *Store) StartTurn(sessionID, provider, model, message string) (*ChatTurn, error) {
	t := &ChatTurn{ID: newID(), SessionID: sessionID, Status: TurnRunning, Provider: provider,
		Model: model, Message: message, StartedAt: time.Now().UTC()}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock the session row so two requests can't both start a turn.
		var cs ChatSession
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", sessionID).First(&cs).Error; err != nil {
			return err
		}
		var running int
		if err := tx.Model(&ChatTurn{}).Where("session_id = ? AND status = ?", sessionID, TurnRunning).
			Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrTurnRunning
		}
		return tx.Create(t).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ErrTurnRunning means the session already has a running turn.
var ErrTurnRunning = errors.New("the assistant is still working on the last message")

// FinishTurn records a turn's end.
func (s *Store) FinishTurn(id, status, errText string) error {
	now := time.Now().UTC()
	return s.db.Model(&ChatTurn{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "error": errText, "finished_at": &now}).Error
}

// Turn returns a turn of a session.
func (s *Store) Turn(sessionID, id string) (*ChatTurn, error) {
	var t ChatTurn
	err := s.db.Where("id = ? AND session_id = ?", id, sessionID).First(&t).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) turnByID(id string) (*ChatTurn, error) {
	var t ChatTurn
	err := s.db.Where("id = ?", id).First(&t).Error
	return &t, err
}

// LatestTurn returns a session's most recent turn, or ErrNotFound.
func (s *Store) LatestTurn(sessionID string) (*ChatTurn, error) {
	var t ChatTurn
	err := s.db.Where("session_id = ?", sessionID).Order("started_at DESC").First(&t).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, ErrNotFound
	}
	return &t, err
}

// FailInterrupted marks turns left running by a previous process as
// failed: their goroutines died with it.
func (s *Store) FailInterrupted() (int64, error) {
	now := time.Now().UTC()
	r := s.db.Model(&ChatTurn{}).Where("status = ?", TurnRunning).Updates(map[string]any{
		"status": TurnFailed, "error": "the server restarted while the assistant was working; send the message again",
		"finished_at": &now})
	return r.RowsAffected, r.Error
}

// AppendEvent adds an event to a turn and sets its id.
func (s *Store) AppendEvent(e *ChatEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return s.db.Create(e).Error
}

// Events returns a turn's events after the id, oldest first.
func (s *Store) Events(turnID string, after uint) ([]ChatEvent, error) {
	var out []ChatEvent
	err := s.db.Where("turn_id = ? AND id > ?", turnID, after).Order("id").Find(&out).Error
	return out, err
}

// Keys returns a user's keys, without their secrets.
func (s *Store) Keys(owner string) ([]LLMKey, error) {
	out := []LLMKey{}
	err := s.db.Where("owner = ?", owner).Order("provider").Find(&out).Error
	return out, err
}

// Key returns a user's key for a provider, or their most recently updated
// key when provider is "".
func (s *Store) Key(owner, provider string) (*LLMKey, error) {
	q := s.db.Where("owner = ?", owner)
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	var k LLMKey
	err := q.Order("updated_at DESC").First(&k).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, ErrNotFound
	}
	return &k, err
}

// PutKey adds or replaces a user's key for its provider.
func (s *Store) PutKey(k *LLMKey) error {
	var existing LLMKey
	err := s.db.Where("owner = ? AND provider = ?", k.Owner, k.Provider).First(&existing).Error
	switch {
	case gorm.IsRecordNotFoundError(err):
		return s.db.Create(k).Error
	case err != nil:
		return err
	}
	k.ID, k.CreatedAt = existing.ID, existing.CreatedAt
	return s.db.Save(k).Error
}

// DeleteKey removes a user's key for a provider.
func (s *Store) DeleteKey(owner, provider string) error {
	r := s.db.Where("owner = ? AND provider = ?", owner, provider).Delete(&LLMKey{})
	if r.Error == nil && r.RowsAffected == 0 {
		return ErrNotFound
	}
	return r.Error
}

func isDuplicate(err error) bool {
	return strings.Contains(err.Error(), "Duplicate entry")
}

func newID() string { return uuid.NewV4().String() }
