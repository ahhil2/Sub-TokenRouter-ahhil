package httpapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewOpenAITextHandlerCopiesNotificationUsers mutates the caller's options
// after constructing a handler. The handler's enabled user scope must remain
// fixed at construction, including when another handler shares the input slice.
func TestNewOpenAITextHandlerCopiesNotificationUsers(t *testing.T) {
	options := OpenAITextOptions{CodexClientNotificationUserIDs: []int64{40}}
	first := NewOpenAITextHandler(options, nil, nil, nil)
	second := NewOpenAITextHandler(options, nil, nil, nil)
	options.CodexClientNotificationUserIDs[0] = 41
	require.Equal(t, []int64{40}, first.options.CodexClientNotificationUserIDs)
	require.Equal(t, []int64{40}, second.options.CodexClientNotificationUserIDs)
	first.options.CodexClientNotificationUserIDs[0] = 42
	require.Equal(t, []int64{40}, second.options.CodexClientNotificationUserIDs)
	require.Empty(t, NewOpenAITextHandler(OpenAITextOptions{}, nil, nil, nil).options.CodexClientNotificationUserIDs)
}
