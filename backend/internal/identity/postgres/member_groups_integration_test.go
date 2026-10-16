//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
)

func TestMemberGroupTransactions(t *testing.T) {
	dsn := os.Getenv("TOKENROUTER_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated identity database")
	}
	require.Contains(t, strings.ToLower(dsn), "identity_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, database)))
	store := NewMemberGroupStore(client)
	ctx := context.Background()
	var userID int64
	require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES('identity@test.local','hash','identity') RETURNING id`).Scan(&userID))
	require.NoError(t, store.ChangeMembers(ctx, userID, "bonus_5", []int64{userID}, true))
	require.NoError(t, store.ChangeMembers(ctx, userID, "ahhilai", []int64{userID}, true))
	require.NoError(t, store.ChangeMembers(ctx, userID, "bonus_10", []int64{userID}, true))
	memberships, err := store.Memberships(ctx, []int64{userID})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ahhilai", "bonus_10"}, memberships[userID])
	require.NoError(t, store.ChangeMembers(ctx, userID, "bonus_5", []int64{userID}, false))
	code, err := store.RechargeIdentity(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "bonus_10", code)
	require.Error(t, store.ChangeMembers(ctx, userID, "bonus_5", []int64{userID, userID + 99999}, true))
	code, err = store.RechargeIdentity(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "bonus_10", code)
	var workers sync.WaitGroup
	failures := make(chan error, 2)
	for _, identityCode := range []string{"bonus_5", "bonus_10"} {
		workers.Add(1)
		go func(code string) {
			defer workers.Done()
			failures <- store.ChangeMembers(ctx, userID, code, []int64{userID}, true)
		}(identityCode)
	}
	workers.Wait()
	close(failures)
	for failure := range failures {
		require.NoError(t, failure)
	}
	memberships, err = store.Memberships(ctx, []int64{userID})
	require.NoError(t, err)
	require.Len(t, memberships[userID], 2)
	members, total, err := store.ListMembers(ctx, "ahhilai", "identity", 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, members, 1)
	require.NoError(t, store.ChangeMembers(ctx, userID, "ahhilai", []int64{userID}, false))
	groups, err := store.ListGroups(ctx)
	require.NoError(t, err)
	for _, group := range groups {
		if group.Code == "ahhilai" {
			require.Zero(t, group.MemberCount)
		}
	}
	var audits int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM identity_group_changes`).Scan(&audits))
	require.Equal(t, 7, audits)
}
