//go:build integration

package jackchat_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/jackchat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/jackchatkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/jackmigrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestChatStoreKeysAndOwnership(t *testing.T) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("chat_test"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))

	var alice, bob, group int64
	for i, dst := range []*int64{&alice, &bob} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES($1,'test','user','active') RETURNING id`, fmt.Sprintf("chat-%d@example.test", i)).Scan(dst))
	}
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,status) VALUES('chat-group','openai','standard','active') RETURNING id`).Scan(&group))

	store := jackchat.NewStore(db)
	settings, err := store.Settings(ctx)
	require.NoError(t, err)
	require.True(t, settings.Enabled)

	// Conversations are private to their owner.
	conv, err := store.CreateConversation(ctx, alice, jackchat.Conversation{Mode: jackchat.ModeChat, GroupID: &group, Model: "gpt-5", ImageAspect: "1:1", ImageCount: 1}, 2)
	require.NoError(t, err)
	_, err = store.Conversation(ctx, bob, conv.ID)
	require.ErrorIs(t, err, jackchat.ErrNotFound)
	_, err = store.DeleteConversation(ctx, bob, conv.ID)
	require.ErrorIs(t, err, jackchat.ErrNotFound)
	_, err = store.CreateConversation(ctx, alice, jackchat.Conversation{Mode: jackchat.ModeChat, ImageAspect: "1:1", ImageCount: 1}, 2)
	require.NoError(t, err)
	_, err = store.CreateConversation(ctx, alice, jackchat.Conversation{Mode: jackchat.ModeChat, ImageAspect: "1:1", ImageCount: 1}, 2)
	require.ErrorIs(t, err, jackchat.ErrTooMany)

	// Attachments: another user's upload cannot be bound; reused images are
	// copied and the shared object is only orphaned when the last row goes.
	bobUpload, err := store.InsertAttachment(ctx, bob, jackchat.Attachment{Kind: jackchat.KindImage, StorageKey: "k/bob.png", Mime: "image/png"})
	require.NoError(t, err)
	msg, err := store.InsertMessage(ctx, jackchat.Message{ConversationID: conv.ID, Role: jackchat.RoleUser, Content: "hi", Status: jackchat.StatusComplete})
	require.NoError(t, err)
	_, err = store.BindAttachments(ctx, alice, msg.ID, []int64{bobUpload.ID})
	require.ErrorIs(t, err, jackchat.ErrBadAttachment)

	reply, err := store.InsertMessage(ctx, jackchat.Message{ConversationID: conv.ID, Role: jackchat.RoleAssistant, Status: jackchat.StatusComplete})
	require.NoError(t, err)
	generated, err := store.InsertAttachment(ctx, alice, jackchat.Attachment{MessageID: &reply.ID, Kind: jackchat.KindGenerated, StorageKey: "k/gen.png", Mime: "image/png"})
	require.NoError(t, err)
	other, err := store.CreateConversation(ctx, alice, jackchat.Conversation{Mode: jackchat.ModeImage, ImageAspect: "1:1", ImageCount: 1}, 0)
	require.NoError(t, err)
	next, err := store.InsertMessage(ctx, jackchat.Message{ConversationID: other.ID, Role: jackchat.RoleUser, Content: "edit", Status: jackchat.StatusComplete})
	require.NoError(t, err)
	bound, err := store.BindAttachments(ctx, alice, next.ID, []int64{generated.ID})
	require.NoError(t, err)
	require.Len(t, bound, 1)
	require.NotEqual(t, generated.ID, bound[0].ID)
	require.Equal(t, jackchat.KindImage, bound[0].Kind)

	orphans, err := store.DeleteConversation(ctx, alice, conv.ID)
	require.NoError(t, err)
	require.Empty(t, orphans, "the reused image is still referenced")
	orphans, err = store.DeleteConversation(ctx, alice, other.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"k/gen.png"}, orphans)

	// Messages keep citations.
	conv, err = store.CreateConversation(ctx, alice, jackchat.Conversation{Mode: jackchat.ModeChat, ImageAspect: "1:1", ImageCount: 1}, 0)
	require.NoError(t, err)
	_, err = store.InsertMessage(ctx, jackchat.Message{ConversationID: conv.ID, Role: jackchat.RoleUser, Content: "q", Status: jackchat.StatusComplete})
	require.NoError(t, err)
	a, err := store.InsertMessage(ctx, jackchat.Message{ConversationID: conv.ID, Role: jackchat.RoleAssistant, Status: jackchat.StatusStreaming, WebSearch: true})
	require.NoError(t, err)
	a.Content, a.Status, a.Citations = "answer", jackchat.StatusComplete, []jackchat.Citation{{URL: "https://example.test", Title: "Ex"}}
	require.NoError(t, store.FinishMessage(ctx, a))
	msgs, err := store.Messages(ctx, conv.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.True(t, msgs[1].WebSearch)
	require.Equal(t, a.Citations, msgs[1].Citations)
	orphans, err = store.DeleteTrailingAssistant(ctx, alice, conv.ID)
	require.NoError(t, err)
	require.Empty(t, orphans)
	msgs, err = store.Messages(ctx, conv.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	// Hidden keys: one per (user, group), invisible to key lists and counts.
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	keyRepo := repository.NewAPIKeyRepository(client, db)
	userRepo := repository.NewUserRepository(client, db)
	keys := service.NewAPIKeyService(keyRepo, userRepo, nil, nil, nil, nil, &config.Config{})
	svc := jackchat.NewService(store, keys, keyRepo, userRepo, nil)
	var wg sync.WaitGroup
	got := make([]string, 4)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			k, e := svc.EnsureKey(ctx, alice, group)
			require.NoError(t, e)
			got[i] = k
		}(i)
	}
	wg.Wait()
	for _, k := range got {
		require.Equal(t, got[0], k)
		require.True(t, jackchatkey.IsChatKey(k))
	}
	require.NoError(t, keyRepo.Create(ctx, &service.APIKey{UserID: alice, Key: "sk-visible-0123456789", Name: "mine", GroupID: &group, Status: service.StatusActive}))
	listed, _, err := keyRepo.ListByUserID(ctx, alice, pagination.PaginationParams{Page: 1, PageSize: 50}, service.APIKeyListFilters{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "sk-visible-0123456789", listed[0].Key)
	count, err := keyRepo.CountByUserID(ctx, alice)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	bobKey, err := svc.EnsureKey(ctx, bob, group)
	require.NoError(t, err)
	require.NotEqual(t, got[0], bobKey)
}

func TestChatUnreadWebSearchAndContextStart(t *testing.T) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("chat_unread"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))

	var user, other int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES('u@example.test','test','user','active') RETURNING id`).Scan(&user))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES('o@example.test','test','user','active') RETURNING id`).Scan(&other))
	store := jackchat.NewStore(db)

	settings, err := store.Settings(ctx)
	require.NoError(t, err)
	require.Equal(t, jackchat.DefaultContextTokens, settings.MaxContextTokens)

	conv, err := store.CreateConversation(ctx, user, jackchat.Conversation{Mode: jackchat.ModeChat, ImageAspect: "1:1", ImageCount: 1, WebSearch: true}, 0)
	require.NoError(t, err)
	require.True(t, conv.WebSearch)
	require.False(t, conv.Unread, "a new conversation has nothing unread")

	require.NoError(t, store.MarkReplied(ctx, conv.ID))
	n, err := store.UnreadCount(ctx, user)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	got, err := store.Conversation(ctx, user, conv.ID)
	require.NoError(t, err)
	require.True(t, got.Unread)

	require.ErrorIs(t, store.MarkRead(ctx, other, conv.ID, false), jackchat.ErrNotFound, "other users cannot change it")
	require.NoError(t, store.MarkRead(ctx, user, conv.ID, false))
	got, err = store.Conversation(ctx, user, conv.ID)
	require.NoError(t, err)
	require.False(t, got.Unread)
	require.NoError(t, store.MarkRead(ctx, user, conv.ID, true))
	got, err = store.Conversation(ctx, user, conv.ID)
	require.NoError(t, err)
	require.True(t, got.Unread, "marked unread by hand")

	got.WebSearch = false
	got, err = store.UpdateConversation(ctx, user, got)
	require.NoError(t, err)
	require.False(t, got.WebSearch)

	require.NoError(t, store.SetContextStart(ctx, conv.ID, 42))
	got, err = store.Conversation(ctx, user, conv.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ContextStartID)
	require.Equal(t, int64(42), *got.ContextStartID)
}
