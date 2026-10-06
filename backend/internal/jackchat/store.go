// Package jackchat stores Jack's web chat conversations. Model calls go through
// the normal gateway with hidden per-(user, group) API keys, so billing,
// scheduling and usage logs stay upstream-owned.
package jackchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

const (
	ModeChat  = "chat"
	ModeImage = "image"

	RoleUser      = "user"
	RoleAssistant = "assistant"

	StatusStreaming = "streaming"
	StatusComplete  = "complete"
	StatusError     = "error"
	StatusAborted   = "aborted"

	KindImage     = "image"
	KindPDF       = "pdf"
	KindText      = "text"
	KindGenerated = "generated"
)

var (
	ErrNotFound      = apperrors.NotFound("CHAT_NOT_FOUND", "conversation or attachment not found")
	ErrDisabled      = apperrors.Forbidden("CHAT_DISABLED", "chat mode is disabled")
	ErrTooMany       = apperrors.BadRequest("CHAT_LIMIT", "conversation limit reached; delete old conversations first")
	ErrBadAttachment = apperrors.BadRequest("CHAT_ATTACHMENT_INVALID", "attachment is not available")
	ErrEmpty         = apperrors.BadRequest("CHAT_EMPTY", "nothing to send")
)

type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

func NewStore(db *sql.DB) *Store { return &Store{DB: db, Now: time.Now} }

type Settings struct {
	Enabled          bool   `json:"enabled"`
	SystemPrompt     string `json:"system_prompt"`
	MaxImageBytes    int64  `json:"max_image_bytes"`
	MaxPDFBytes      int64  `json:"max_pdf_bytes"`
	MaxTextBytes     int64  `json:"max_text_bytes"`
	MaxAttachments   int    `json:"max_attachments"`
	MaxConversations int    `json:"max_conversations"`
}

type Preference struct {
	Mode            string `json:"mode"`
	GroupID         *int64 `json:"group_id"`
	ChatModel       string `json:"chat_model"`
	ReasoningEffort string `json:"reasoning_effort"`
	ImageModel      string `json:"image_model"`
	ImageAspect     string `json:"image_aspect"`
	ImageCount      int    `json:"image_count"`
	WebSearch       bool   `json:"web_search"`
}

type Conversation struct {
	ID              int64     `json:"id"`
	Mode            string    `json:"mode"`
	GroupID         *int64    `json:"group_id"`
	Model           string    `json:"model"`
	ReasoningEffort string    `json:"reasoning_effort"`
	ImageAspect     string    `json:"image_aspect"`
	ImageCount      int       `json:"image_count"`
	Title           string    `json:"title"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Attachment struct {
	ID        int64     `json:"id"`
	MessageID *int64    `json:"message_id,omitempty"`
	Kind      string    `json:"kind"`
	Filename  string    `json:"filename"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	StorageKey    string `json:"-"`
	ExtractedText string `json:"-"`
}

type Message struct {
	ID              int64        `json:"id"`
	ConversationID  int64        `json:"conversation_id"`
	Role            string       `json:"role"`
	Content         string       `json:"content"`
	Reasoning       string       `json:"reasoning,omitempty"`
	Model           string       `json:"model,omitempty"`
	ReasoningEffort string       `json:"reasoning_effort,omitempty"`
	Status          string       `json:"status"`
	Error           string       `json:"error,omitempty"`
	InputTokens     int          `json:"input_tokens"`
	OutputTokens    int          `json:"output_tokens"`
	WebSearch       bool         `json:"web_search,omitempty"`
	Citations       []Citation   `json:"citations"`
	CreatedAt       time.Time    `json:"created_at"`
	Attachments     []Attachment `json:"attachments"`
}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var out Settings
	err := s.DB.QueryRowContext(ctx, `SELECT enabled, system_prompt, max_image_bytes, max_pdf_bytes, max_text_bytes, max_attachments, max_conversations FROM jack_chat_settings WHERE id=1`).
		Scan(&out.Enabled, &out.SystemPrompt, &out.MaxImageBytes, &out.MaxPDFBytes, &out.MaxTextBytes, &out.MaxAttachments, &out.MaxConversations)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{Enabled: true, MaxImageBytes: 20 << 20, MaxPDFBytes: 32 << 20, MaxTextBytes: 2 << 20, MaxAttachments: 10, MaxConversations: 500}, nil
	}
	return out, err
}

func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	if in.MaxImageBytes <= 0 || in.MaxPDFBytes <= 0 || in.MaxTextBytes <= 0 || in.MaxAttachments <= 0 || in.MaxConversations <= 0 {
		return Settings{}, apperrors.BadRequest("CHAT_SETTINGS_INVALID", "limits must be positive")
	}
	if in.MaxImageBytes > 64<<20 || in.MaxPDFBytes > 64<<20 || in.MaxTextBytes > 16<<20 || in.MaxAttachments > 32 {
		return Settings{}, apperrors.BadRequest("CHAT_SETTINGS_INVALID", "limits are too large")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO jack_chat_settings(id, enabled, system_prompt, max_image_bytes, max_pdf_bytes, max_text_bytes, max_attachments, max_conversations, updated_at)
VALUES (1,$1,$2,$3,$4,$5,$6,$7,now())
ON CONFLICT (id) DO UPDATE SET enabled=EXCLUDED.enabled, system_prompt=EXCLUDED.system_prompt, max_image_bytes=EXCLUDED.max_image_bytes,
 max_pdf_bytes=EXCLUDED.max_pdf_bytes, max_text_bytes=EXCLUDED.max_text_bytes, max_attachments=EXCLUDED.max_attachments,
 max_conversations=EXCLUDED.max_conversations, updated_at=now()`,
		in.Enabled, in.SystemPrompt, in.MaxImageBytes, in.MaxPDFBytes, in.MaxTextBytes, in.MaxAttachments, in.MaxConversations)
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

func (s *Store) Preference(ctx context.Context, userID int64) (Preference, error) {
	out := Preference{Mode: ModeChat, ImageAspect: "1:1", ImageCount: 1}
	var group sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT mode, group_id, chat_model, reasoning_effort, image_model, image_aspect, image_count, web_search FROM jack_chat_preferences WHERE user_id=$1`, userID).
		Scan(&out.Mode, &group, &out.ChatModel, &out.ReasoningEffort, &out.ImageModel, &out.ImageAspect, &out.ImageCount, &out.WebSearch)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if group.Valid {
		out.GroupID = &group.Int64
	}
	return out, err
}

func (s *Store) SavePreference(ctx context.Context, userID int64, p Preference) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO jack_chat_preferences(user_id, mode, group_id, chat_model, reasoning_effort, image_model, image_aspect, image_count, web_search, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())
ON CONFLICT (user_id) DO UPDATE SET mode=EXCLUDED.mode, group_id=EXCLUDED.group_id, chat_model=EXCLUDED.chat_model, reasoning_effort=EXCLUDED.reasoning_effort,
 image_model=EXCLUDED.image_model, image_aspect=EXCLUDED.image_aspect, image_count=EXCLUDED.image_count, web_search=EXCLUDED.web_search, updated_at=now()`,
		userID, p.Mode, nullInt(p.GroupID), p.ChatModel, p.ReasoningEffort, p.ImageModel, p.ImageAspect, p.ImageCount, p.WebSearch)
	return err
}

const conversationColumns = `id, mode, group_id, model, reasoning_effort, image_aspect, image_count, title, created_at, updated_at`

func scanConversation(row interface{ Scan(...any) error }) (Conversation, error) {
	var c Conversation
	var group sql.NullInt64
	err := row.Scan(&c.ID, &c.Mode, &group, &c.Model, &c.ReasoningEffort, &c.ImageAspect, &c.ImageCount, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if group.Valid {
		c.GroupID = &group.Int64
	}
	return c, err
}

func (s *Store) ListConversations(ctx context.Context, userID int64) ([]Conversation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+conversationColumns+` FROM jack_chat_conversations WHERE user_id=$1 ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Conversation(ctx context.Context, userID, id int64) (Conversation, error) {
	c, err := scanConversation(s.DB.QueryRowContext(ctx, `SELECT `+conversationColumns+` FROM jack_chat_conversations WHERE user_id=$1 AND id=$2`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateConversation(ctx context.Context, userID int64, in Conversation, limit int) (Conversation, error) {
	if limit > 0 {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM jack_chat_conversations WHERE user_id=$1`, userID).Scan(&n); err != nil {
			return Conversation{}, err
		}
		if n >= limit {
			return Conversation{}, ErrTooMany
		}
	}
	return scanConversation(s.DB.QueryRowContext(ctx, `INSERT INTO jack_chat_conversations(user_id, mode, group_id, model, reasoning_effort, image_aspect, image_count, title)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+conversationColumns,
		userID, in.Mode, nullInt(in.GroupID), in.Model, in.ReasoningEffort, in.ImageAspect, in.ImageCount, in.Title))
}

func (s *Store) UpdateConversation(ctx context.Context, userID int64, c Conversation) (Conversation, error) {
	out, err := scanConversation(s.DB.QueryRowContext(ctx, `UPDATE jack_chat_conversations SET mode=$3, group_id=$4, model=$5, reasoning_effort=$6, image_aspect=$7, image_count=$8, title=$9, updated_at=now()
WHERE user_id=$1 AND id=$2 RETURNING `+conversationColumns,
		userID, c.ID, c.Mode, nullInt(c.GroupID), c.Model, c.ReasoningEffort, c.ImageAspect, c.ImageCount, c.Title))
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return out, err
}

func (s *Store) TouchConversation(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jack_chat_conversations SET updated_at=now() WHERE id=$1`, id)
	return err
}

// DeleteConversation removes the conversation and returns the storage keys
// that no other attachment row still references.
func (s *Store) DeleteConversation(ctx context.Context, userID, id int64) ([]string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := collectKeys(ctx, tx, `SELECT a.storage_key FROM jack_chat_attachments a JOIN jack_chat_messages m ON m.id=a.message_id WHERE m.conversation_id=$1 AND a.user_id=$2 AND a.storage_key<>''`, id, userID)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM jack_chat_conversations WHERE user_id=$1 AND id=$2`, userID, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	orphans, err := orphanKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	return orphans, tx.Commit()
}

const messageColumns = `id, conversation_id, role, content, reasoning, model, reasoning_effort, status, error, input_tokens, output_tokens, web_search, citations, created_at`

func (s *Store) Messages(ctx context.Context, conversationID int64) ([]Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+messageColumns+` FROM jack_chat_messages WHERE conversation_id=$1 ORDER BY id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Message{}
	index := map[int64]int{}
	ids := []int64{}
	for rows.Next() {
		var m Message
		var citations []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Reasoning, &m.Model, &m.ReasoningEffort, &m.Status, &m.Error, &m.InputTokens, &m.OutputTokens, &m.WebSearch, &citations, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Citations = parseCitations(citations)
		m.Attachments = []Attachment{}
		index[m.ID] = len(out)
		ids = append(ids, m.ID)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}
	arows, err := s.DB.QueryContext(ctx, `SELECT `+attachmentColumns+` FROM jack_chat_attachments WHERE message_id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = arows.Close() }()
	for arows.Next() {
		a, err := scanAttachment(arows)
		if err != nil {
			return nil, err
		}
		if a.MessageID != nil {
			if i, ok := index[*a.MessageID]; ok {
				out[i].Attachments = append(out[i].Attachments, a)
			}
		}
	}
	return out, arows.Err()
}

func (s *Store) InsertMessage(ctx context.Context, m Message) (Message, error) {
	err := s.DB.QueryRowContext(ctx, `INSERT INTO jack_chat_messages(conversation_id, role, content, reasoning, model, reasoning_effort, status, error, web_search)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at`,
		m.ConversationID, m.Role, m.Content, m.Reasoning, m.Model, m.ReasoningEffort, m.Status, m.Error, m.WebSearch).Scan(&m.ID, &m.CreatedAt)
	if m.Attachments == nil {
		m.Attachments = []Attachment{}
	}
	return m, err
}

func (s *Store) FinishMessage(ctx context.Context, m Message) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jack_chat_messages SET content=$2, reasoning=$3, status=$4, error=$5, input_tokens=$6, output_tokens=$7, citations=$8 WHERE id=$1`,
		m.ID, m.Content, m.Reasoning, m.Status, m.Error, m.InputTokens, m.OutputTokens, citationsJSON(m.Citations))
	return err
}

// DeleteTrailingAssistant removes the final assistant reply of a conversation
// (used by regenerate) and returns orphaned storage keys.
func (s *Store) DeleteTrailingAssistant(ctx context.Context, userID, conversationID int64) ([]string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	var role string
	err = tx.QueryRowContext(ctx, `SELECT id, role FROM jack_chat_messages WHERE conversation_id=$1 ORDER BY id DESC LIMIT 1 FOR UPDATE`, conversationID).Scan(&id, &role)
	if errors.Is(err, sql.ErrNoRows) || role != RoleAssistant {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	keys, err := collectKeys(ctx, tx, `SELECT storage_key FROM jack_chat_attachments WHERE message_id=$1 AND user_id=$2 AND storage_key<>''`, id, userID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM jack_chat_messages WHERE id=$1`, id); err != nil {
		return nil, err
	}
	orphans, err := orphanKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	return orphans, tx.Commit()
}

const attachmentColumns = `id, message_id, kind, storage_key, filename, mime, size, width, height, extracted_text, created_at`

func scanAttachment(row interface{ Scan(...any) error }) (Attachment, error) {
	var a Attachment
	var msg sql.NullInt64
	err := row.Scan(&a.ID, &msg, &a.Kind, &a.StorageKey, &a.Filename, &a.Mime, &a.Size, &a.Width, &a.Height, &a.ExtractedText, &a.CreatedAt)
	if msg.Valid {
		a.MessageID = &msg.Int64
	}
	return a, err
}

func (s *Store) InsertAttachment(ctx context.Context, userID int64, a Attachment) (Attachment, error) {
	err := s.DB.QueryRowContext(ctx, `INSERT INTO jack_chat_attachments(user_id, message_id, kind, storage_key, filename, mime, size, width, height, extracted_text)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, created_at`,
		userID, nullInt(a.MessageID), a.Kind, a.StorageKey, a.Filename, a.Mime, a.Size, a.Width, a.Height, a.ExtractedText).Scan(&a.ID, &a.CreatedAt)
	return a, err
}

func (s *Store) Attachment(ctx context.Context, userID, id int64) (Attachment, error) {
	a, err := scanAttachment(s.DB.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM jack_chat_attachments WHERE user_id=$1 AND id=$2`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return a, err
}

// BindAttachments links the user's attachments to messageID. Pending uploads
// are moved; attachments that already belong to a message (for example a
// generated image reused as a reference) are copied so each message owns its
// own rows while sharing the stored object.
func (s *Store) BindAttachments(ctx context.Context, userID, messageID int64, ids []int64) ([]Attachment, error) {
	if len(ids) == 0 {
		return []Attachment{}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := make([]Attachment, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		a, err := scanAttachment(tx.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM jack_chat_attachments WHERE user_id=$1 AND id=$2 FOR UPDATE`, userID, id))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBadAttachment
		}
		if err != nil {
			return nil, err
		}
		if a.MessageID == nil {
			if _, err = tx.ExecContext(ctx, `UPDATE jack_chat_attachments SET message_id=$2 WHERE id=$1`, a.ID, messageID); err != nil {
				return nil, err
			}
		} else {
			kind := a.Kind
			if kind == KindGenerated {
				kind = KindImage
			}
			if err = tx.QueryRowContext(ctx, `INSERT INTO jack_chat_attachments(user_id, message_id, kind, storage_key, filename, mime, size, width, height, extracted_text)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, created_at`,
				userID, messageID, kind, a.StorageKey, a.Filename, a.Mime, a.Size, a.Width, a.Height, a.ExtractedText).Scan(&a.ID, &a.CreatedAt); err != nil {
				return nil, err
			}
			a.Kind = kind
		}
		a.MessageID = &messageID
		out = append(out, a)
	}
	return out, tx.Commit()
}

// DeletePendingAttachment removes an upload not yet sent with a message.
func (s *Store) DeletePendingAttachment(ctx context.Context, userID, id int64) ([]string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var key string
	err = tx.QueryRowContext(ctx, `DELETE FROM jack_chat_attachments WHERE user_id=$1 AND id=$2 AND message_id IS NULL RETURNING storage_key`, userID, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	orphans, err := orphanKeys(ctx, tx, []string{key})
	if err != nil {
		return nil, err
	}
	return orphans, tx.Commit()
}

// StalePending removes uploads that were never sent and returns orphaned keys.
func (s *Store) StalePending(ctx context.Context, before time.Time) ([]string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := collectKeys(ctx, tx, `DELETE FROM jack_chat_attachments WHERE message_id IS NULL AND created_at < $1 RETURNING storage_key`, before)
	if err != nil {
		return nil, err
	}
	orphans, err := orphanKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	return orphans, tx.Commit()
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func collectKeys(ctx context.Context, q queryer, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	return out, rows.Err()
}

func orphanKeys(ctx context.Context, q queryer, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT k FROM unnest($1::text[]) AS k WHERE NOT EXISTS (SELECT 1 FROM jack_chat_attachments a WHERE a.storage_key=k)`, pq.Array(dedupe(keys)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func nullInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// Citation is a web source cited by an assistant reply.
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

func parseCitations(raw []byte) []Citation {
	out := []Citation{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []Citation{}
	}
	return out
}

func citationsJSON(c []Citation) string {
	if len(c) == 0 {
		return "[]"
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "[]"
	}
	return string(b)
}
