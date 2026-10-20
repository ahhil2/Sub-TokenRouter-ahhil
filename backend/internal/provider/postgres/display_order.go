package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

func (r *ProviderStore) UpdateDisplayOrder(ctx context.Context, ids []int64) error {
	transaction, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	result, err := transaction.Client().ExecContext(ctx, `
		UPDATE providers AS target
		SET extra = COALESCE(target.extra, '{}'::jsonb) || jsonb_build_object('display_order', ordered.sort_index),
		    updated_at = NOW()
		FROM unnest($1::bigint[]) WITH ORDINALITY AS ordered(id, sort_index)
		WHERE target.id = ordered.id AND target.deleted_at IS NULL
	`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("update provider display order: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return provider.ErrProviderNotFound
	}
	return transaction.Commit()
}
