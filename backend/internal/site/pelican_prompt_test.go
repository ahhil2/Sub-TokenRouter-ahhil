package site

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPelicanPromptConditions(testContext *testing.T) {
	first := pelicanPromptWithConditions([8]byte{})
	second := pelicanPromptWithConditions([8]byte{2, 5, 3, 8, 2, 1, 4, 6})
	require.True(testContext, strings.HasPrefix(first, PelicanPrompt))
	require.NotEqual(testContext, first, second)
	for _, expected := range []string{"场景：樱花小巷", "服装：侦探风衣", "车子：折叠车", "视觉风格：霓虹赛博朋克", "时间：黄昏", "天气：细雨", "配色：紫罗兰与湖蓝", "故事：带着望远镜去观星"} {
		require.Contains(testContext, second, expected)
	}
	for index := range 256 {
		value := byte(index)
		prompt := pelicanPromptWithConditions([8]byte{value, value, value, value, value, value, value, value})
		require.Contains(testContext, prompt, "完整 HTML")
	}
}

func TestNewPelicanGenerationPrompt(testContext *testing.T) {
	first, err := NewPelicanGenerationPrompt()
	require.NoError(testContext, err)
	second, err := NewPelicanGenerationPrompt()
	require.NoError(testContext, err)
	third, err := NewPelicanGenerationPrompt()
	require.NoError(testContext, err)
	require.False(testContext, first == second && second == third)
}
