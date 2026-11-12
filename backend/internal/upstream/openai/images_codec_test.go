package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// images_codec_test.go checks image-tool animation parameters during OAuth
// request conversion. The existing Responses driver remains independent of the
// selected image model while normalized GIF controls reach the image tool.

// TestBuildOpenAIImagesResponsesRequestGIF preserves normalized animation controls
// through OAuth conversion. Responses lifecycle streaming remains enabled while
// the image tool carries no partial-image or compression controls.
func TestBuildOpenAIImagesResponsesRequestGIF(t *testing.T) {
	parsed, err := upstream.ParseImageRequest("/v1/images/generations", "application/json", []byte(`{"model":"gpt-image-2.6-flare","prompt":"A looping light","output_format":"gif","frame_count":4,"frame_duration_ms":255}`))
	require.NoError(t, err)
	body, err := BuildOpenAIImagesResponsesRequest(parsed, parsed.Model)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(body, "stream").Bool())
	require.Equal(t, "gif", gjson.GetBytes(body, "tools.0.output_format").String())
	require.Equal(t, int64(4), gjson.GetBytes(body, "tools.0.frame_count").Int())
	require.Equal(t, int64(260), gjson.GetBytes(body, "tools.0.frame_duration_ms").Int())
	require.False(t, gjson.GetBytes(body, "tools.0.partial_images").Exists())
}
