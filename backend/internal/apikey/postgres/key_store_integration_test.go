//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestHasBatchImageKeyMatchesVisibleActiveKeys varies key status, ownership, protocol
// permission, management, and deletion in PostgreSQL. It verifies the existence query
// grants menu eligibility only for a matching visible active key.
func TestHasBatchImageKeyMatchesVisibleActiveKeys(t *testing.T) {
	db := postgrescontainer.New(t)
	for _, test := range []struct {
		name         string // Case display label.
		status       string // Stored key status.
		protocols    string // Group protocol JSON.
		managed      bool   // System-managed key flag.
		deletedKey   bool   // Soft-deleted key flag.
		deletedGroup bool   // Soft-deleted group flag.
		otherOwner   bool   // Different authenticated owner.
		allowed      bool   // Expected menu eligibility.
	}{
		{name: "allowed", status: "active", protocols: `["image_batches"]`, allowed: true},
		{name: "disabled", status: "disabled", protocols: `["image_batches"]`},
		{name: "no_protocol", status: "active", protocols: `["openai_responses"]`},
		{name: "managed", status: "active", protocols: `["image_batches"]`, managed: true},
		{name: "deleted_key", status: "active", protocols: `["image_batches"]`, deletedKey: true},
		{name: "deleted_group", status: "active", protocols: `["image_batches"]`, deletedGroup: true},
		{name: "other_owner", status: "active", protocols: `["image_batches"]`, otherOwner: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			// Roll back each fixture transaction before the next case so ownership and
			// protocol observations stay isolated while sharing the same database container.
			t.Cleanup(func() { require.NoError(t, tx.Rollback()) })
			var owner, other, groupID int64
			require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('owner@example.test','test') RETURNING id").Scan(&owner))
			require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('other@example.test','test') RETURNING id").Scan(&other))
			require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO groups(name,allowed_protocols) VALUES('images',$1::jsonb) RETURNING id", test.protocols).Scan(&groupID))
			keyOwner := owner
			if test.otherOwner {
				keyOwner = other
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id,status,managed_by,deleted_at)
                VALUES($1,'sk-fixture','fixture',$2,$3,CASE WHEN $4 THEN 'creative_studio' ELSE NULL END, CASE WHEN $5 THEN NOW() ELSE NULL END)`,
				keyOwner, groupID, test.status, test.managed, test.deletedKey)
			require.NoError(t, err)
			if test.deletedGroup {
				_, err = tx.ExecContext(ctx, "UPDATE groups SET deleted_at=NOW() WHERE id=$1", groupID)
				require.NoError(t, err)
			}
			allowed, err := NewKeyStoreWithSQL(nil, tx, nil).HasBatchImageKey(ctx, owner)
			require.NoError(t, err)
			require.Equal(t, test.allowed, allowed)
		})
	}
}
