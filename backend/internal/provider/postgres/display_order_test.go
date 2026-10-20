package postgres

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
)

func TestUpdateDisplayOrderTransaction(test *testing.T) {
	for _, scenario := range []struct {
		name    string
		count   int64
		failure error
	}{
		{name: "commits all providers", count: 2},
		{name: "rolls back missing provider", count: 1},
		{name: "rolls back database failure", failure: errors.New("database failure")},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			require.NoError(test, err)
			defer func() { _ = database.Close() }()
			client := ent.NewClient(ent.Driver(sql.OpenDB(dialect.Postgres, database)))
			store := NewProviderStore(client, database, ProviderStoreOptions{})
			mock.ExpectBegin()
			update := mock.ExpectExec("UPDATE providers AS target").WithArgs("{2,1}")
			if scenario.failure != nil {
				update.WillReturnError(scenario.failure)
			} else {
				update.WillReturnResult(sqlmock.NewResult(0, scenario.count))
			}
			if scenario.count == 2 {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = store.UpdateDisplayOrder(context.Background(), []int64{2, 1})
			if scenario.count == 2 {
				require.NoError(test, err)
			} else {
				require.Error(test, err)
			}
			require.NoError(test, mock.ExpectationsWereMet())
		})
	}
}

func TestProviderDisplayOrderQuery(test *testing.T) {
	selector := sql.Dialect(dialect.Postgres).Select("id").From(sql.Table("providers"))
	for _, order := range ProviderListOrder(pagination.PaginationParams{SortBy: "display_order", SortOrder: "asc"}) {
		order(selector)
	}
	query, _ := selector.Query()
	require.Contains(test, query, "jsonb_typeof(extra->'display_order') = 'number'")
	require.Contains(test, query, "::numeric END ASC NULLS LAST")
	require.NotContains(test, query, "priority")
}
