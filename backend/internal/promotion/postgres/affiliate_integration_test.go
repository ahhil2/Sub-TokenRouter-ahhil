//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
)

func TestHistoricalAffiliateRelationships(test *testing.T) {
	dsn := os.Getenv("TOKENROUTER_AFFILIATE_TEST_DSN")
	if dsn == "" {
		test.Skip("requires isolated affiliate_test database")
	}
	require.Contains(test, strings.ToLower(dsn), "/affiliate_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(test, err)
	database.SetMaxOpenConns(1)
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, database)))
	test.Cleanup(func() { _ = client.Close() })
	_, err = database.Exec(`CREATE TEMP TABLE user_affiliates (
		user_id bigint PRIMARY KEY, inviter_id bigint, aff_count integer DEFAULT 0,
		created_at timestamptz DEFAULT '2025-01-01');
		INSERT INTO user_affiliates(user_id, inviter_id, aff_count) VALUES
		(1,NULL,0), (2,1,0), (3,1,0), (4,2,0), (5,NULL,127), (6,NULL,0), (7,99,0)`)
	require.NoError(test, err)
	repository := NewAffiliateRepository(client, nil)
	relationships, err := repository.GetRelationships(context.Background(), []int64{1, 2, 5, 6, 99, 100})
	require.NoError(test, err)
	require.Len(test, relationships, 6)
	require.Equal(test, int64(2), relationships[1].AffCount)
	require.Equal(test, int64(1), *relationships[2].InviterID)
	require.Equal(test, int64(1), relationships[2].AffCount)
	require.Equal(test, int64(127), relationships[5].AffCount)
	require.Nil(test, relationships[6].InviterID)
	require.Zero(test, relationships[6].AffCount)
	require.Equal(test, int64(1), relationships[99].AffCount)
	require.Nil(test, relationships[100].InviterID)
	require.Zero(test, relationships[100].AffCount)

	_, err = database.Exec(`INSERT INTO user_affiliates(user_id, inviter_id, created_at) VALUES(8,1,NOW())`)
	require.NoError(test, err)
	updated, err := repository.GetRelationships(context.Background(), []int64{1})
	require.NoError(test, err)
	require.Equal(test, int64(3), updated[1].AffCount)
	var savedCount int
	require.NoError(test, database.QueryRow(`SELECT aff_count FROM user_affiliates WHERE user_id = 1`).Scan(&savedCount))
	require.Zero(test, savedCount)
}
