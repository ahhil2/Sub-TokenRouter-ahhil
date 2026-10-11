package postgres

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/site"
)

type messageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) site.MessageRepository {
	return &messageRepository{db: db}
}

func (repository *messageRepository) Unread(ctx context.Context, actor site.MessageActor) (int64, error) {
	var count int64
	err := repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_messages message
		JOIN site_message_conversations conversation ON conversation.user_id = message.user_id
		JOIN users recipient ON recipient.id = message.user_id
		WHERE ($2 OR message.user_id = $1) AND message.is_admin <> $2
		AND recipient.deleted_at IS NULL AND recipient.role = 'user'
		AND message.id > CASE WHEN $2 THEN conversation.admin_read_message_id ELSE conversation.user_read_message_id END`, actor.ID, actor.Admin).Scan(&count)
	return count, err
}

func (repository *messageRepository) Conversations(ctx context.Context, actor site.MessageActor, search string, page int) (site.MessagePage[site.MessageConversation], error) {
	result := site.MessagePage[site.MessageConversation]{Items: []site.MessageConversation{}}
	rows, err := repository.db.QueryContext(ctx, `SELECT conversation.user_id, COALESCE(recipient.username, ''), recipient.email,
		message.id, message.body, message.is_admin, message.created_at,
		(SELECT COUNT(*) FROM site_messages unread WHERE unread.user_id = conversation.user_id AND unread.is_admin <> $2
		AND unread.id > CASE WHEN $2 THEN conversation.admin_read_message_id ELSE conversation.user_read_message_id END)
		FROM site_message_conversations conversation JOIN users recipient ON recipient.id = conversation.user_id
		JOIN site_messages message ON message.id = conversation.last_message_id
		WHERE ($2 OR conversation.user_id = $1) AND recipient.deleted_at IS NULL AND recipient.role = 'user'
		AND ($3 = '' OR recipient.email ILIKE '%' || $3 || '%' OR recipient.username ILIKE '%' || $3 || '%')
		ORDER BY conversation.last_message_id DESC LIMIT 31 OFFSET $4`, actor.ID, actor.Admin, search, (page-1)*30)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item site.MessageConversation
		if err := rows.Scan(&item.UserID, &item.Username, &item.Email, &item.LastMessageID, &item.Body, &item.IsAdmin, &item.CreatedAt, &item.UnreadCount); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > 30 {
		result.Items = result.Items[:30]
		result.HasMore = true
	}
	return result, nil
}

func (repository *messageRepository) Contacts(ctx context.Context, search string, page int) (site.MessagePage[site.MessageContact], error) {
	result := site.MessagePage[site.MessageContact]{Items: []site.MessageContact{}}
	rows, err := repository.db.QueryContext(ctx, `SELECT id, COALESCE(username, ''), email FROM users
		WHERE deleted_at IS NULL AND status = 'active' AND role = 'user'
		AND ($1 = '' OR email ILIKE '%' || $1 || '%' OR username ILIKE '%' || $1 || '%')
		ORDER BY id LIMIT 31 OFFSET $2`, search, (page-1)*30)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var contact site.MessageContact
		if err := rows.Scan(&contact.ID, &contact.Username, &contact.Email); err != nil {
			return result, err
		}
		result.Items = append(result.Items, contact)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > 30 {
		result.Items = result.Items[:30]
		result.HasMore = true
	}
	return result, nil
}

func (repository *messageRepository) Thread(ctx context.Context, actor site.MessageActor, userID, before int64) (*site.MessageThread, error) {
	if !actor.CanAccess(userID) {
		return nil, site.ErrMessageForbidden
	}
	result := &site.MessageThread{MessagePage: site.MessagePage[site.Message]{Items: []site.Message{}}}
	var userRead, adminRead int64
	err := repository.db.QueryRowContext(ctx, `SELECT recipient.id, COALESCE(recipient.username, ''), recipient.email,
		COALESCE(conversation.user_read_message_id, 0), COALESCE(conversation.admin_read_message_id, 0)
		FROM users recipient LEFT JOIN site_message_conversations conversation ON conversation.user_id = recipient.id
		WHERE recipient.id = $1 AND recipient.deleted_at IS NULL AND recipient.role = 'user'`, userID).
		Scan(&result.Contact.ID, &result.Contact.Username, &result.Contact.Email, &userRead, &adminRead)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, site.ErrMessageNotFound
	}
	if err != nil {
		return nil, err
	}
	result.ReadThroughID, result.PeerReadThroughID = userRead, adminRead
	if actor.Admin {
		result.ReadThroughID, result.PeerReadThroughID = adminRead, userRead
	}
	rows, err := repository.db.QueryContext(ctx, `SELECT id, user_id, sender_id, is_admin, body, created_at FROM site_messages
		WHERE user_id = $1 AND ($2::bigint = 0 OR id < $2) ORDER BY id DESC LIMIT 51`, userID, before)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var message site.Message
		if err := rows.Scan(&message.ID, &message.UserID, &message.SenderID, &message.IsAdmin, &message.Body, &message.CreatedAt); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > 50 {
		result.Items = result.Items[:50]
		result.HasMore = true
	}
	slices.Reverse(result.Items)
	return result, nil
}

func (repository *messageRepository) MarkRead(ctx context.Context, actor site.MessageActor, userID, through int64) error {
	if !actor.CanAccess(userID) {
		return site.ErrMessageForbidden
	}
	result, err := repository.db.ExecContext(ctx, `UPDATE site_message_conversations SET
		user_read_message_id = CASE WHEN $2 THEN user_read_message_id ELSE GREATEST(user_read_message_id, $3) END,
		admin_read_message_id = CASE WHEN $2 THEN GREATEST(admin_read_message_id, $3) ELSE admin_read_message_id END
		WHERE user_id = $1 AND EXISTS (SELECT 1 FROM site_messages WHERE user_id = $1 AND id = $3)`, userID, actor.Admin, through)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return site.ErrMessageNotFound
	}
	return nil
}

func (repository *messageRepository) Send(ctx context.Context, actor site.MessageActor, request site.SendMessageRequest, payloadHash string) (int, error) {
	if actor.ID <= 0 || !actor.Admin && (request.AllUsers || len(request.RecipientIDs) != 1 || request.RecipientIDs[0] != actor.ID) {
		return 0, site.ErrMessageForbidden
	}
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()
	var role string
	err = transaction.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1 AND deleted_at IS NULL AND status = 'active' FOR SHARE`, actor.ID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (role == "admin") != actor.Admin {
		return 0, site.ErrMessageForbidden
	}
	if err != nil {
		return 0, err
	}
	_, err = transaction.ExecContext(ctx, `INSERT INTO site_message_dispatches (sender_id, client_id, payload_hash)
		VALUES ($1, $2, $3) ON CONFLICT (sender_id, client_id) DO NOTHING`, actor.ID, request.ClientID, payloadHash)
	if err != nil {
		return 0, err
	}
	var dispatchID int64
	var savedHash string
	var count int
	err = transaction.QueryRowContext(ctx, `SELECT id, payload_hash, recipient_count FROM site_message_dispatches
		WHERE sender_id = $1 AND client_id = $2 FOR UPDATE`, actor.ID, request.ClientID).Scan(&dispatchID, &savedHash, &count)
	if err != nil {
		return 0, err
	}
	if savedHash != payloadHash {
		return 0, site.ErrMessageConflict
	}
	if count > 0 {
		return count, transaction.Commit()
	}
	rows, err := transaction.QueryContext(ctx, `SELECT id FROM users WHERE deleted_at IS NULL AND status = 'active' AND role = 'user'
		AND ($1 OR id = ANY($2)) ORDER BY id LIMIT $3 FOR SHARE`, request.AllUsers, pq.Array(request.RecipientIDs), site.MaxMessageBroadcastRecipients+1)
	if err != nil {
		return 0, err
	}
	var recipients []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		recipients = append(recipients, userID)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return 0, err
	}
	if len(recipients) == 0 || !request.AllUsers && len(recipients) != len(request.RecipientIDs) {
		return 0, site.ErrMessageRecipient
	}
	if len(recipients) > site.MaxMessageBroadcastRecipients {
		return 0, site.ErrMessageInvalid
	}
	_, err = transaction.ExecContext(ctx, `INSERT INTO site_message_conversations (user_id)
		SELECT unnest($1::bigint[]) ON CONFLICT (user_id) DO NOTHING`, pq.Array(recipients))
	if err != nil {
		return 0, err
	}
	locked, err := transaction.QueryContext(ctx, `SELECT user_id FROM site_message_conversations WHERE user_id = ANY($1) ORDER BY user_id FOR UPDATE`, pq.Array(recipients))
	if err != nil {
		return 0, err
	}
	for locked.Next() {
		var userID int64
		if err := locked.Scan(&userID); err != nil {
			return 0, errors.Join(err, locked.Close())
		}
	}
	err = errors.Join(locked.Err(), locked.Close())
	if err != nil {
		return 0, err
	}
	for _, userID := range recipients {
		var messageID int64
		err = transaction.QueryRowContext(ctx, `INSERT INTO site_messages (user_id, sender_id, is_admin, dispatch_id, body)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, userID, actor.ID, actor.Admin, dispatchID, request.Body).Scan(&messageID)
		if err != nil {
			return 0, err
		}
		_, err = transaction.ExecContext(ctx, `UPDATE site_message_conversations SET last_message_id = $2 WHERE user_id = $1`, userID, messageID)
		if err != nil {
			return 0, err
		}
	}
	_, err = transaction.ExecContext(ctx, `UPDATE site_message_dispatches SET recipient_count = $2 WHERE id = $1`, dispatchID, len(recipients))
	if err != nil {
		return 0, err
	}
	return len(recipients), transaction.Commit()
}
