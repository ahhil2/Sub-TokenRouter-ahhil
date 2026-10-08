package site

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageSendCapture struct {
	MessageRepository
	request SendMessageRequest
	actor   MessageActor
	hash    string
	calls   int
}

func (capture *messageSendCapture) Send(_ context.Context, actor MessageActor, request SendMessageRequest, hash string) (int, error) {
	capture.actor, capture.request, capture.hash = actor, request, hash
	capture.calls++
	return len(request.RecipientIDs), nil
}

func TestMessageSendValidation(t *testing.T) {
	valid := SendMessageRequest{Body: "Hello 😊", ClientID: "019bb611-0872-7000-8000-000000000001", RecipientIDs: []int64{2}}
	tests := []struct {
		name   string
		actor  MessageActor
		change func(*SendMessageRequest)
		want   error
	}{
		{"anonymous", MessageActor{}, func(*SendMessageRequest) {}, ErrMessageForbidden},
		{"blank", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.Body = " \n\t " }, ErrMessageInvalid},
		{"too long", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.Body = strings.Repeat("😊", MaxMessageLength+1) }, ErrMessageInvalid},
		{"null byte", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.Body = "hello\x00" }, ErrMessageInvalid},
		{"invalid utf8", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.Body = string([]byte{0xff}) }, ErrMessageInvalid},
		{"missing idempotency key", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.ClientID = "" }, ErrMessageInvalid},
		{"nil uuid", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.ClientID = "00000000-0000-0000-0000-000000000000" }, ErrMessageInvalid},
		{"no recipients", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.RecipientIDs = nil }, ErrMessageInvalid},
		{"ambiguous broadcast", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.AllUsers = true }, ErrMessageInvalid},
		{"negative recipient", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.RecipientIDs = []int64{-2} }, ErrMessageInvalid},
		{"self recipient", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.RecipientIDs = []int64{1} }, ErrMessageInvalid},
		{"too many recipients", MessageActor{ID: 1, Admin: true}, func(request *SendMessageRequest) { request.RecipientIDs = make([]int64, MaxMessageRecipients+1) }, ErrMessageInvalid},
		{"user targets another user", MessageActor{ID: 3}, func(*SendMessageRequest) {}, ErrMessageForbidden},
		{"user broadcasts", MessageActor{ID: 3}, func(request *SendMessageRequest) { request.RecipientIDs = nil; request.AllUsers = true }, ErrMessageForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &messageSendCapture{}
			request := valid
			test.change(&request)
			_, err := NewMessageService(repository).Send(context.Background(), test.actor, request)
			require.ErrorIs(t, err, test.want)
			assert.Zero(t, repository.calls)
		})
	}
}

func TestMessageRecipientsAndEmoji(t *testing.T) {
	repository := &messageSendCapture{}
	service := NewMessageService(repository)
	request := SendMessageRequest{Body: "  你好 👨‍👩‍👧‍👦 ❤️\nHello  ", ClientID: "019bb611-0872-7000-8000-000000000001", RecipientIDs: []int64{3, 2, 3}}
	count, err := service.Send(context.Background(), MessageActor{ID: 1, Admin: true}, request)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, []int64{2, 3}, repository.request.RecipientIDs)
	assert.Equal(t, []int64{3, 2, 3}, request.RecipientIDs)
	assert.Equal(t, "你好 👨‍👩‍👧‍👦 ❤️\nHello", repository.request.Body)
	hash := repository.hash
	request.RecipientIDs = []int64{2, 3}
	_, err = service.Send(context.Background(), MessageActor{ID: 1, Admin: true}, request)
	require.NoError(t, err)
	assert.Equal(t, hash, repository.hash)
	request.RecipientIDs = nil
	request.Body = strings.Repeat("😊", MaxMessageLength)
	_, err = service.Send(context.Background(), MessageActor{ID: 3}, request)
	require.NoError(t, err)
	assert.Equal(t, []int64{3}, repository.request.RecipientIDs)
	assert.False(t, repository.actor.Admin)
}

func TestMessageUserIsolation(t *testing.T) {
	service := NewMessageService(nil)
	actor := MessageActor{ID: 2}
	_, err := service.Thread(context.Background(), actor, 3, 0)
	require.ErrorIs(t, err, ErrMessageForbidden)
	require.ErrorIs(t, service.MarkRead(context.Background(), actor, 3, 1), ErrMessageForbidden)
	_, err = service.Contacts(context.Background(), actor, "", 1)
	require.ErrorIs(t, err, ErrMessageForbidden)
	_, err = service.Conversations(context.Background(), actor, "", 0)
	require.ErrorIs(t, err, ErrMessageInvalid)
}

func TestMessageBroadcastNormalizesEmptyRecipients(t *testing.T) {
	repository := &messageSendCapture{}
	service := NewMessageService(repository)
	request := SendMessageRequest{Body: "Hello", ClientID: "019bb611-0872-7000-8000-000000000001", AllUsers: true}
	actor := MessageActor{ID: 1, Admin: true}
	_, err := service.Send(context.Background(), actor, request)
	require.NoError(t, err)
	hash := repository.hash
	request.RecipientIDs = []int64{}
	_, err = service.Send(context.Background(), actor, request)
	require.NoError(t, err)
	assert.Equal(t, hash, repository.hash)
}

func TestMessageSearchValidation(t *testing.T) {
	service := NewMessageService(nil)
	actor := MessageActor{ID: 1, Admin: true}
	for _, search := range []string{"hello\x00", string([]byte{0xff}), strings.Repeat("字", 129)} {
		_, err := service.Contacts(context.Background(), actor, search, 1)
		require.ErrorIs(t, err, ErrMessageInvalid)
		_, err = service.Conversations(context.Background(), actor, search, 1)
		require.ErrorIs(t, err, ErrMessageInvalid)
	}
}
