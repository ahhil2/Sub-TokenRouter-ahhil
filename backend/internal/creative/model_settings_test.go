package creative

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeCreativeModelSettings 检查白名单字段校验、去重和排序。
func TestNormalizeCreativeModelSettings(t *testing.T) {
	settings, err := NormalizeCreativeModelSettings([]CreativeModelSetting{{
		GroupID:    12,
		Model:      " gemini-3.1-flash-image ",
		Operations: []string{"inpaint", "generate", "generate"},
	}})
	require.NoError(t, err)
	require.Equal(t, []CreativeModelSetting{{
		GroupID:    12,
		Model:      "gemini-3.1-flash-image",
		Operations: []string{"generate", "inpaint"},
	}}, settings)

	for name, input := range map[string][]CreativeModelSetting{
		"非正整数分组": {{GroupID: 0, Model: "image", Operations: []string{CreativeOperationGenerate}}},
		"空模型":    {{GroupID: 1, Model: " ", Operations: []string{CreativeOperationGenerate}}},
		"空能力":    {{GroupID: 1, Model: "image", Operations: nil}},
		"非法能力":   {{GroupID: 1, Model: "image", Operations: []string{"upscale"}}},
		"重复模型": {
			{GroupID: 1, Model: "image", Operations: []string{CreativeOperationGenerate}},
			{GroupID: 1, Model: "image", Operations: []string{CreativeOperationEdit}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeCreativeModelSettings(input)
			require.Error(t, err)
		})
	}
}
