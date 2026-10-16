package identity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type memberGroupTestStore struct {
	MemberGroupRepository
	ids  []int64
	code string
	add  bool
}

func (store *memberGroupTestStore) ChangeMembers(_ context.Context, _ int64, code string, ids []int64, add bool) error {
	store.ids, store.code, store.add = ids, code, add
	return nil
}

func TestMemberGroupsChange(t *testing.T) {
	store := &memberGroupTestStore{}
	invalidated := []int64{}
	service := &MemberGroups{Repository: store, Invalidate: func(_ context.Context, id int64) { invalidated = append(invalidated, id) }}
	require.NoError(t, service.Change(context.Background(), 1, "bonus_10", []int64{3, 2, 3}, true))
	require.Equal(t, []int64{2, 3}, store.ids)
	require.Equal(t, store.ids, invalidated)
	require.Equal(t, "bonus_10", store.code)
	require.True(t, store.add)
	require.Error(t, service.Change(context.Background(), 1, "unknown", []int64{2}, true))
	require.Error(t, service.Change(context.Background(), 1, "ahhilai", nil, true))
	require.Error(t, service.Change(context.Background(), 1, "ahhilai", []int64{-1}, true))
	require.NoError(t, service.Change(context.Background(), 1, "ahhilai", []int64{2}, false))
	require.False(t, store.add)
}

func TestMemberGroupCategories(t *testing.T) {
	for _, code := range []string{"bonus_5", "bonus_10"} {
		category, err := MemberGroupCategory(code)
		require.NoError(t, err)
		require.Equal(t, "recharge", category)
	}
	category, err := MemberGroupCategory("ahhilai")
	require.NoError(t, err)
	require.Equal(t, "pricing", category)
}
