//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/site"
	"github.com/TokenFlux/TokenRouter/migrations"
)

func TestMessageRepository(t *testing.T) {
	dsn := os.Getenv("SITE_MESSAGES_TEST_DSN")
	if dsn == "" {
		t.Skip("SITE_MESSAGES_TEST_DSN must point to an isolated messages_test database")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "/messages_test", parsed.Path)
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	ctx := context.Background()
	require.NoError(t, postgresinfra.ApplyMigrations(ctx, database, migrations.FS))
	_, err = database.ExecContext(ctx, `TRUNCATE site_messages, site_message_dispatches, site_message_conversations RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	ids := make([]int64, 5)
	t.Cleanup(func() {
		_, err := database.ExecContext(ctx, `DELETE FROM users WHERE id = ANY($1)`, pq.Array(ids))
		require.NoError(t, err)
	})
	for index, role := range []string{"admin", "admin", "user", "user", "user"} {
		status := "active"
		if index == 4 {
			status = "disabled"
		}
		err := database.QueryRowContext(ctx, `INSERT INTO users (email, username, password_hash, role, status) VALUES ($1, $2, 'test-only', $3, $4) RETURNING id`, uuid.NewString()+"@example.invalid", fmt.Sprintf("Message test %d", index), role, status).Scan(&ids[index])
		require.NoError(t, err)
	}
	admin := site.MessageActor{ID: ids[0], Admin: true}
	otherAdmin := site.MessageActor{ID: ids[1], Admin: true}
	user := site.MessageActor{ID: ids[2]}
	otherUser := site.MessageActor{ID: ids[3]}
	service := site.NewMessageService(NewMessageRepository(database))
	t.Run("repository rejects user recipient overrides", func(t *testing.T) {
		for _, request := range []site.SendMessageRequest{
			{RecipientIDs: []int64{otherUser.ID}},
			{AllUsers: true},
		} {
			_, err := NewMessageRepository(database).Send(ctx, user, request, "")
			require.ErrorIs(t, err, site.ErrMessageForbidden)
		}
	})
	reset := func(t *testing.T) {
		t.Helper()
		_, err := database.ExecContext(ctx, `TRUNCATE site_messages, site_message_dispatches, site_message_conversations RESTART IDENTITY CASCADE`)
		require.NoError(t, err)
	}
	t.Run("private threads replies and read receipts", func(t *testing.T) {
		reset(t)
		body := "你好 😊 ❤️\n<script>alert('text only')</script>"
		count, err := service.Send(ctx, admin, site.SendMessageRequest{Body: body, ClientID: uuid.NewString(), RecipientIDs: []int64{user.ID, otherUser.ID}})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		list, err := service.Conversations(ctx, user, "", 1)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		assert.Equal(t, user.ID, list.Items[0].UserID)
		thread, err := service.Thread(ctx, user, user.ID, 0)
		require.NoError(t, err)
		require.Len(t, thread.Items, 1)
		assert.Equal(t, body, thread.Items[0].Body)
		_, err = service.Thread(ctx, user, otherUser.ID, 0)
		require.ErrorIs(t, err, site.ErrMessageForbidden)
		peer, err := service.Thread(ctx, otherUser, otherUser.ID, 0)
		require.NoError(t, err)
		require.ErrorIs(t, service.MarkRead(ctx, user, user.ID, peer.Items[0].ID), site.ErrMessageNotFound)
		require.NoError(t, service.MarkRead(ctx, user, user.ID, thread.Items[0].ID))
		_, err = service.Send(ctx, user, site.SendMessageRequest{Body: "收到，谢谢 👍", ClientID: uuid.NewString()})
		require.NoError(t, err)
		unread, err := service.Unread(ctx, admin)
		require.NoError(t, err)
		assert.EqualValues(t, 1, unread)
		adminThread, err := service.Thread(ctx, otherAdmin, user.ID, 0)
		require.NoError(t, err)
		assert.Equal(t, thread.Items[0].ID, adminThread.PeerReadThroughID)
		require.NoError(t, service.MarkRead(ctx, otherAdmin, user.ID, adminThread.Items[1].ID))
		unread, err = service.Unread(ctx, admin)
		require.NoError(t, err)
		assert.Zero(t, unread)
		_, err = service.Send(ctx, admin, site.SendMessageRequest{Body: "New message", ClientID: uuid.NewString(), RecipientIDs: []int64{user.ID}})
		require.NoError(t, err)
		require.NoError(t, service.MarkRead(ctx, user, user.ID, thread.Items[0].ID))
		unread, err = service.Unread(ctx, user)
		require.NoError(t, err)
		assert.EqualValues(t, 1, unread)
	})
	t.Run("concurrent retries deliver once and reject changed payload", func(t *testing.T) {
		reset(t)
		request := site.SendMessageRequest{Body: "Idempotent 🎉", ClientID: uuid.NewString(), RecipientIDs: []int64{user.ID}}
		start := make(chan struct{})
		var wait sync.WaitGroup
		errors := make([]error, 2)
		counts := make([]int, 2)
		for index := range 2 {
			wait.Go(func() { <-start; counts[index], errors[index] = service.Send(ctx, admin, request) })
		}
		close(start)
		wait.Wait()
		for index := range 2 {
			require.NoError(t, errors[index])
			assert.Equal(t, 1, counts[index])
		}
		thread, err := service.Thread(ctx, user, user.ID, 0)
		require.NoError(t, err)
		assert.Len(t, thread.Items, 1)
		request.Body = "Different message"
		_, err = service.Send(ctx, admin, request)
		require.ErrorIs(t, err, site.ErrMessageConflict)
	})
	t.Run("invalid recipients roll back the entire dispatch", func(t *testing.T) {
		reset(t)
		_, err := service.Send(ctx, admin, site.SendMessageRequest{Body: "Do not partially deliver", ClientID: uuid.NewString(), RecipientIDs: []int64{user.ID, ids[4]}})
		require.ErrorIs(t, err, site.ErrMessageRecipient)
		var count int
		require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_message_dispatches`).Scan(&count))
		assert.Zero(t, count)
		thread, err := service.Thread(ctx, user, user.ID, 0)
		require.NoError(t, err)
		assert.Empty(t, thread.Items)
	})
	t.Run("cursor pagination has no duplicate or missing messages", func(t *testing.T) {
		reset(t)
		for index := range 51 {
			_, err := service.Send(ctx, admin, site.SendMessageRequest{Body: fmt.Sprintf("Message %02d", index), ClientID: uuid.NewString(), RecipientIDs: []int64{user.ID}})
			require.NoError(t, err)
		}
		latest, err := service.Thread(ctx, user, user.ID, 0)
		require.NoError(t, err)
		require.Len(t, latest.Items, 50)
		assert.True(t, latest.HasMore)
		older, err := service.Thread(ctx, user, user.ID, latest.Items[0].ID)
		require.NoError(t, err)
		require.Len(t, older.Items, 1)
		assert.False(t, older.HasMore)
		assert.Less(t, older.Items[0].ID, latest.Items[0].ID)
		assert.Equal(t, "Message 00", older.Items[0].Body)
		assert.Equal(t, "Message 50", latest.Items[49].Body)
	})
	t.Run("broadcast retries retain their original recipient snapshot", func(t *testing.T) {
		reset(t)
		request := site.SendMessageRequest{Body: "Broadcast 📢", ClientID: uuid.NewString(), AllUsers: true}
		count, err := service.Send(ctx, admin, request)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		var newUserID int64
		err = database.QueryRowContext(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'test-only') RETURNING id`, uuid.NewString()+"@example.invalid").Scan(&newUserID)
		require.NoError(t, err)
		ids = append(ids, newUserID)
		count, err = service.Send(ctx, admin, request)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		thread, err := service.Thread(ctx, admin, newUserID, 0)
		require.NoError(t, err)
		assert.Empty(t, thread.Items)
		contacts, err := service.Contacts(ctx, admin, "example.invalid", 1)
		require.NoError(t, err)
		for _, contact := range contacts.Items {
			assert.NotContains(t, []int64{admin.ID, otherAdmin.ID, ids[4]}, contact.ID)
			assert.True(t, strings.HasSuffix(contact.Email, "@example.invalid"))
		}
	})
}
