package postgres

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
)

func TestGetRelationships(test *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(test, err)
	client := ent.NewClient(ent.Driver(sql.OpenDB(dialect.Postgres, database)))
	test.Cleanup(func() { _ = client.Close() })
	repository := NewAffiliateRepository(client, nil)
	empty, err := repository.GetRelationships(context.Background(), nil)
	require.NoError(test, err)
	require.Empty(test, empty)
	mock.ExpectQuery(regexp.QuoteMeta(affiliateRelationshipsSQL)).WithArgs("{1,2,3,4}").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "inviter_id", "count"}).
			AddRow(1, nil, 0).AddRow(2, 9001, 0).AddRow(3, nil, 127).AddRow(4, 2, 8)).RowsWillBeClosed()
	relationships, err := repository.GetRelationships(context.Background(), []int64{1, 2, 3, 4})
	require.NoError(test, err)
	require.Len(test, relationships, 4)
	require.Nil(test, relationships[1].InviterID)
	require.Zero(test, relationships[1].AffCount)
	require.Equal(test, int64(9001), *relationships[2].InviterID)
	require.Equal(test, int64(127), relationships[3].AffCount)
	require.Equal(test, int64(2), *relationships[4].InviterID)
	require.Equal(test, int64(8), relationships[4].AffCount)
	require.NoError(test, mock.ExpectationsWereMet())
}

func TestGetRelationshipsErrors(test *testing.T) {
	for _, failure := range []string{"query", "scan", "rows"} {
		test.Run(failure, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			require.NoError(test, err)
			client := ent.NewClient(ent.Driver(sql.OpenDB(dialect.Postgres, database)))
			test.Cleanup(func() { _ = client.Close() })
			expected := mock.ExpectQuery(regexp.QuoteMeta(affiliateRelationshipsSQL)).WithArgs("{1}")
			switch failure {
			case "query":
				expected.WillReturnError(errors.New("database unavailable"))
			case "scan":
				expected.WillReturnRows(sqlmock.NewRows([]string{"user_id", "inviter_id", "count"}).AddRow(1, nil, "invalid")).RowsWillBeClosed()
			case "rows":
				expected.WillReturnRows(sqlmock.NewRows([]string{"user_id", "inviter_id", "count"}).AddRow(1, nil, 0).RowError(0, errors.New("read failed"))).RowsWillBeClosed()
			}
			relationships, err := NewAffiliateRepository(client, nil).GetRelationships(context.Background(), []int64{1})
			require.Error(test, err)
			require.Nil(test, relationships)
			require.NoError(test, mock.ExpectationsWereMet())
		})
	}
}

func TestAffiliateUserOverviewSQLIncludesMaturedFrozenQuota(t *testing.T) {
	query := strings.Join(strings.Fields(affiliateUserOverviewSQL), " ")

	require.Contains(t, query, "ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0)")
	require.Contains(t, query, "frozen_until <= NOW()")
}

func TestAffiliateRecordQueriesUseLedgerAuditFields(t *testing.T) {
	source, err := os.ReadFile("affiliate.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "JOIN payment_orders po ON po.id = ual.source_order_id")
	require.Contains(t, content, "LEFT JOIN subscription_plans sp ON sp.id = po.plan_id")
	require.Contains(t, content, "ual.amount::double precision")
	require.Contains(t, content, "COALESCE(NULLIF(po.order_type, ''), 'balance')")
	require.Contains(t, content, "NULLIF(UPPER(po.plan_snapshot->>'currency'), '')")
	require.Contains(t, content, "NULLIF(UPPER(sp.currency), '')")
	require.Contains(t, content, "COALESCE(NULLIF(UPPER(po.provider_snapshot->>'currency'), ''), 'CNY')")
	require.Contains(t, content, "ual.balance_after::double precision")
	require.NotContains(t, content, "parseAffiliateRebateAmount")
	require.NotContains(t, content, `"current_balance": "u.balance"`)
}
