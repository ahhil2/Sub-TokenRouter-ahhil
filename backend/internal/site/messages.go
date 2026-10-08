package site

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	MaxMessageLength              = 4000
	MaxMessageRecipients          = 100
	MaxMessageBroadcastRecipients = 10000
)

var (
	ErrMessageForbidden = apperror.Forbidden("SITE_MESSAGE_FORBIDDEN", "You cannot access this conversation.")
	ErrMessageInvalid   = apperror.BadRequest("SITE_MESSAGE_INVALID", "Check the message and recipients.")
	ErrMessageRecipient = apperror.BadRequest("SITE_MESSAGE_RECIPIENT", "A recipient is unavailable.")
	ErrMessageNotFound  = apperror.NotFound("SITE_MESSAGE_NOT_FOUND", "Message not found.")
	ErrMessageConflict  = apperror.Conflict("SITE_MESSAGE_CONFLICT", "This send request was already used for a different message.")
)

type MessageActor struct {
	ID    int64
	Admin bool
}

type Message struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	SenderID  int64     `json:"sender_id"`
	IsAdmin   bool      `json:"is_admin"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageContact struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type MessageConversation struct {
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	LastMessageID int64     `json:"last_message_id"`
	Body          string    `json:"body"`
	IsAdmin       bool      `json:"is_admin"`
	CreatedAt     time.Time `json:"created_at"`
	UnreadCount   int64     `json:"unread_count"`
}

type MessagePage[T any] struct {
	Items   []T  `json:"items"`
	HasMore bool `json:"has_more"`
}

type MessageThread struct {
	MessagePage[Message]
	Contact           MessageContact `json:"contact"`
	ReadThroughID     int64          `json:"read_through_id"`
	PeerReadThroughID int64          `json:"peer_read_through_id"`
}

type SendMessageRequest struct {
	Body         string  `json:"body"`
	ClientID     string  `json:"client_id"`
	RecipientIDs []int64 `json:"recipient_ids"`
	AllUsers     bool    `json:"all_users"`
}

type MessageRepository interface {
	Unread(context.Context, MessageActor) (int64, error)
	Conversations(context.Context, MessageActor, string, int) (MessagePage[MessageConversation], error)
	Contacts(context.Context, string, int) (MessagePage[MessageContact], error)
	Thread(context.Context, MessageActor, int64, int64) (*MessageThread, error)
	MarkRead(context.Context, MessageActor, int64, int64) error
	Send(context.Context, MessageActor, SendMessageRequest, string) (int, error)
}

type MessageService struct {
	repository MessageRepository
}

func (actor MessageActor) CanAccess(userID int64) bool {
	return actor.ID > 0 && userID > 0 && (actor.Admin || actor.ID == userID)
}

func NewMessageService(repository MessageRepository) *MessageService {
	return &MessageService{repository: repository}
}

func (service *MessageService) Unread(ctx context.Context, actor MessageActor) (int64, error) {
	if actor.ID <= 0 {
		return 0, ErrMessageForbidden
	}
	return service.repository.Unread(ctx, actor)
}

func (service *MessageService) Conversations(ctx context.Context, actor MessageActor, search string, page int) (MessagePage[MessageConversation], error) {
	if actor.ID <= 0 {
		return MessagePage[MessageConversation]{}, ErrMessageForbidden
	}
	if page < 1 || page > 10000 || !utf8.ValidString(search) || strings.ContainsRune(search, 0) || utf8.RuneCountInString(search) > 128 {
		return MessagePage[MessageConversation]{}, ErrMessageInvalid
	}
	return service.repository.Conversations(ctx, actor, strings.TrimSpace(search), page)
}

func (service *MessageService) Contacts(ctx context.Context, actor MessageActor, search string, page int) (MessagePage[MessageContact], error) {
	if actor.ID <= 0 || !actor.Admin {
		return MessagePage[MessageContact]{}, ErrMessageForbidden
	}
	if page < 1 || page > 10000 || !utf8.ValidString(search) || strings.ContainsRune(search, 0) || utf8.RuneCountInString(search) > 128 {
		return MessagePage[MessageContact]{}, ErrMessageInvalid
	}
	return service.repository.Contacts(ctx, strings.TrimSpace(search), page)
}

func (service *MessageService) Thread(ctx context.Context, actor MessageActor, userID, before int64) (*MessageThread, error) {
	if !actor.CanAccess(userID) {
		return nil, ErrMessageForbidden
	}
	if before < 0 {
		return nil, ErrMessageInvalid
	}
	return service.repository.Thread(ctx, actor, userID, before)
}

func (service *MessageService) MarkRead(ctx context.Context, actor MessageActor, userID, through int64) error {
	if !actor.CanAccess(userID) {
		return ErrMessageForbidden
	}
	if through <= 0 {
		return ErrMessageInvalid
	}
	return service.repository.MarkRead(ctx, actor, userID, through)
}

func (service *MessageService) Send(ctx context.Context, actor MessageActor, request SendMessageRequest) (int, error) {
	if actor.ID <= 0 {
		return 0, ErrMessageForbidden
	}
	request.Body = strings.TrimSpace(request.Body)
	clientID, err := uuid.Parse(request.ClientID)
	if err != nil || clientID == uuid.Nil || request.Body == "" || !utf8.ValidString(request.Body) || strings.ContainsRune(request.Body, 0) || utf8.RuneCountInString(request.Body) > MaxMessageLength {
		return 0, ErrMessageInvalid
	}
	request.ClientID = clientID.String()
	if !actor.Admin {
		if request.AllUsers || len(request.RecipientIDs) > 0 {
			return 0, ErrMessageForbidden
		}
		request.RecipientIDs = []int64{actor.ID}
	} else {
		if request.AllUsers == (len(request.RecipientIDs) > 0) || len(request.RecipientIDs) > MaxMessageRecipients {
			return 0, ErrMessageInvalid
		}
		request.RecipientIDs = slices.Clone(request.RecipientIDs)
		for _, userID := range request.RecipientIDs {
			if userID <= 0 || userID == actor.ID {
				return 0, ErrMessageInvalid
			}
		}
		slices.Sort(request.RecipientIDs)
		request.RecipientIDs = slices.Compact(request.RecipientIDs)
		if request.AllUsers {
			request.RecipientIDs = nil
		}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return 0, err
	}
	return service.repository.Send(ctx, actor, request, fmt.Sprintf("%x", sha256.Sum256(payload)))
}
