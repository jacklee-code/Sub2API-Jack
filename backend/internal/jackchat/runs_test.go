package jackchat

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func next(t *testing.T, ch <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case ev, ok := <-ch:
		require.True(t, ok, "channel closed early")
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

func TestRunContinuesAfterDetachAndReattachGetsSnapshot(t *testing.T) {
	runs := NewRuns()
	step := make(chan struct{})
	run, err := runs.Start(1, 10, func(ctx context.Context, emit Emit) error {
		emit(map[string]any{"type": "start", "assistant_message": Message{ID: 5, Status: StatusStreaming}})
		emit(map[string]any{"type": "delta", "text": "Hel"})
		<-step // the page leaves here
		emit(map[string]any{"type": "search", "status": "done", "query": "q"})
		emit(map[string]any{"type": "delta", "text": "lo"})
		<-step
		emit(map[string]any{"type": "done", "message": Message{ID: 5, Content: "Hello", Status: StatusComplete}})
		return nil
	})
	require.NoError(t, err)

	_, events, detach := run.Subscribe()
	require.Equal(t, "start", next(t, events)["type"])
	require.Equal(t, "Hel", next(t, events)["text"])
	detach() // browser navigates away; the run is not cancelled
	step <- struct{}{}

	_, err = runs.Start(1, 10, func(context.Context, Emit) error { return nil })
	require.ErrorIs(t, err, ErrBusy, "one run per conversation")
	require.Nil(t, runs.Get(2, 10), "other users cannot attach")

	require.Eventually(t, func() bool {
		snap, _, d := run.Subscribe()
		defer d()
		return snap != nil && snap.Message.Content == "Hello"
	}, 2*time.Second, 10*time.Millisecond)

	snap, events, detach := runs.Get(1, 10).Subscribe()
	defer detach()
	require.Equal(t, "Hello", snap.Message.Content)
	require.Equal(t, []string{"q"}, snap.SearchQueries)
	step <- struct{}{}
	done := next(t, events)
	require.Equal(t, "done", done["type"])
	_, open := <-events
	require.False(t, open)

	// A page that comes back right after completion still gets the final reply.
	snap, events, _ = runs.Get(1, 10).Subscribe()
	require.Equal(t, StatusComplete, snap.Message.Status)
	require.Equal(t, "done", next(t, events)["type"])
}

func TestRunCancelStopsGenerationAndFailBeforeStartReachesCaller(t *testing.T) {
	runs := NewRuns()
	stopped := make(chan string, 1)
	run, err := runs.Start(1, 20, func(ctx context.Context, emit Emit) error {
		emit(map[string]any{"type": "start", "assistant_message": Message{ID: 7}})
		<-ctx.Done()
		status, _ := runEndStatus(ctx)
		stopped <- status
		emit(map[string]any{"type": "done", "message": Message{ID: 7, Status: status}})
		return nil
	})
	require.NoError(t, err)
	_, events, detach := run.Subscribe()
	defer detach()
	require.Equal(t, "start", next(t, events)["type"])
	run.Cancel()
	require.Equal(t, StatusAborted, <-stopped)
	require.Equal(t, "done", next(t, events)["type"])

	boom := errors.New("insufficient balance")
	failed, err := runs.Start(1, 21, func(context.Context, Emit) error { return boom })
	require.NoError(t, err)
	_, events, _ = failed.Subscribe()
	ev := next(t, events)
	if ev["type"] == "fail" {
		require.Equal(t, boom, ev["error"])
	} else {
		t.Fatalf("expected fail, got %v", ev)
	}
}

func TestClientFromCopiesForwardedHeadersOnly(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/chat/conversations/1/messages", nil)
	r.Header.Set("User-Agent", "browser")
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("Authorization", "Bearer panel-jwt")
	c := ClientFrom(r)
	require.Equal(t, "browser", c.Header.Get("User-Agent"))
	require.Equal(t, "203.0.113.9", c.Header.Get("X-Forwarded-For"))
	require.Empty(t, c.Header.Get("Authorization"), "the panel token never reaches the gateway")
	r.Header.Set("User-Agent", "changed")
	require.Equal(t, "browser", c.Header.Get("User-Agent"))
}
