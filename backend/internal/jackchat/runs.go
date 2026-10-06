package jackchat

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// RunTimeout bounds one generation. Replies still streaming after it (for
// example when the process restarted mid-run) are marked as interrupted.
const RunTimeout = 30 * time.Minute

// runLinger keeps a finished run around so a page that reconnects right after
// completion still receives the final message.
const runLinger = time.Minute

var ErrBusy = apperrors.Conflict("CHAT_BUSY", "this conversation is still generating; wait or stop it first")

// Client is the part of the browser request a background run needs. It is
// copied up front because the request may be gone before the run finishes.
type Client struct {
	RemoteAddr string
	Host       string
	Header     http.Header
}

// ClientFrom copies the forwarded headers of r.
func ClientFrom(r *http.Request) Client {
	c := Client{Header: http.Header{}}
	if r == nil {
		return c
	}
	c.RemoteAddr, c.Host = r.RemoteAddr, r.Host
	for _, h := range forwardedHeaders {
		if v := r.Header.Values(h); len(v) > 0 {
			c.Header[h] = append([]string(nil), v...)
		}
	}
	return c
}

// ImageError is a failed image in an image-mode run.
type ImageError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// RunState is the live view of a reply, sent to pages that (re)attach.
type RunState struct {
	Message       Message      `json:"message"`
	Searching     bool         `json:"searching"`
	SearchQueries []string     `json:"search_queries"`
	PendingImages int          `json:"pending_images"`
	ImageErrors   []ImageError `json:"image_errors"`
	Aspect        string       `json:"aspect,omitempty"`
}

// Run is one generation that keeps going without a browser attached.
type Run struct {
	UserID         int64
	ConversationID int64

	mu       sync.Mutex
	started  bool
	finished bool
	failErr  error
	state    RunState
	subs     map[int]chan map[string]any
	nextSub  int
	cancel   context.CancelFunc
}

// Cancel stops the run; the reply is saved as stopped.
func (r *Run) Cancel() { r.cancel() }

// Subscribe returns the current state (when the reply has started), a channel
// of later events, and a function to detach. The channel closes when the run
// ends or the subscriber falls too far behind.
func (r *Run) Subscribe() (*RunState, <-chan map[string]any, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan map[string]any, 1024)
	var snap *RunState
	if r.started {
		s := r.copyState()
		snap = &s
	}
	if r.finished {
		if r.failErr != nil {
			ch <- map[string]any{"type": "fail", "error": r.failErr}
		} else {
			ch <- map[string]any{"type": "done", "message": r.state.Message}
		}
		close(ch)
		return snap, ch, func() {}
	}
	id := r.nextSub
	r.nextSub++
	r.subs[id] = ch
	return snap, ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if c, ok := r.subs[id]; ok {
			delete(r.subs, id)
			close(c)
		}
	}
}

func (r *Run) copyState() RunState {
	s := r.state
	s.Message.Attachments = append([]Attachment{}, r.state.Message.Attachments...)
	s.Message.Citations = append([]Citation{}, r.state.Message.Citations...)
	s.SearchQueries = append([]string{}, r.state.SearchQueries...)
	s.ImageErrors = append([]ImageError{}, r.state.ImageErrors...)
	return s
}

// publish records ev in the live state and forwards it to subscribers.
func (r *Run) publish(ev map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.apply(ev)
	for id, ch := range r.subs {
		select {
		case ch <- ev:
		default:
			// A stalled page reconnects and gets a fresh snapshot.
			delete(r.subs, id)
			close(ch)
		}
	}
}

func (r *Run) apply(ev map[string]any) {
	s := &r.state
	switch ev["type"] {
	case "start":
		r.started = true
		if m, ok := ev["assistant_message"].(Message); ok {
			s.Message = m
		}
		if n, ok := ev["count"].(int); ok {
			s.PendingImages = n
		}
		if a, ok := ev["aspect"].(string); ok {
			s.Aspect = a
		}
	case "delta":
		if t, ok := ev["text"].(string); ok {
			s.Message.Content += t
		}
		s.Searching = false
	case "reasoning":
		if t, ok := ev["text"].(string); ok {
			s.Message.Reasoning += t
		}
	case "search":
		if ev["status"] == "searching" {
			s.Searching = true
		} else {
			s.Searching = false
			q, _ := ev["query"].(string)
			s.SearchQueries = append(s.SearchQueries, q)
		}
	case "citations":
		if c, ok := ev["citations"].([]Citation); ok {
			s.Message.Citations = append([]Citation{}, c...)
		}
	case "image":
		if a, ok := ev["attachment"].(*Attachment); ok && a != nil {
			s.Message.Attachments = append(s.Message.Attachments, *a)
		}
		if s.PendingImages > 0 {
			s.PendingImages--
		}
	case "image_error":
		i, _ := ev["index"].(int)
		e, _ := ev["error"].(string)
		s.ImageErrors = append(s.ImageErrors, ImageError{Index: i, Error: e})
		if s.PendingImages > 0 {
			s.PendingImages--
		}
	case "done":
		if m, ok := ev["message"].(Message); ok {
			s.Message = m
		}
		s.Searching, s.PendingImages = false, 0
	}
}

func (r *Run) finish(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished = true
	if err != nil && !r.started {
		r.failErr = err
	}
	for id, ch := range r.subs {
		if err != nil {
			if r.started {
				ch <- map[string]any{"type": "error", "message": apperrors.Message(err)}
			} else {
				ch <- map[string]any{"type": "fail", "error": err}
			}
		}
		delete(r.subs, id)
		close(ch)
	}
}

// Runs tracks the active generation of each conversation in this process.
type Runs struct {
	mu     sync.Mutex
	byConv map[int64]*Run
}

func NewRuns() *Runs { return &Runs{byConv: map[int64]*Run{}} }

// Start launches fn in the background. Only one run per conversation may be
// active. fn receives a context that is cancelled by Cancel or RunTimeout,
// never by the browser disconnecting.
func (m *Runs) Start(userID, conversationID int64, fn func(ctx context.Context, emit Emit) error) (*Run, error) {
	m.mu.Lock()
	if old, ok := m.byConv[conversationID]; ok {
		old.mu.Lock()
		busy := !old.finished
		old.mu.Unlock()
		if busy {
			m.mu.Unlock()
			return nil, ErrBusy
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), RunTimeout)
	run := &Run{UserID: userID, ConversationID: conversationID, subs: map[int]chan map[string]any{}, cancel: cancel}
	m.byConv[conversationID] = run
	m.mu.Unlock()

	go func() {
		defer cancel()
		err := fn(ctx, run.publish)
		run.finish(err)
		time.AfterFunc(runLinger, func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.byConv[conversationID] == run {
				delete(m.byConv, conversationID)
			}
		})
	}()
	return run, nil
}

// Get returns the user's run for a conversation, active or just finished.
func (m *Runs) Get(userID, conversationID int64) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.byConv[conversationID]
	if run == nil || run.UserID != userID {
		return nil
	}
	return run
}

// runEndStatus maps a finished run context to the saved reply status.
func runEndStatus(ctx context.Context) (string, string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return StatusError, "generation timed out"
	}
	return StatusAborted, ""
}
