package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/identity"
)

type MemberGroupStore struct{ client *dbent.Client }

func NewMemberGroupStore(client *dbent.Client) *MemberGroupStore {
	return &MemberGroupStore{client: client}
}

func (store *MemberGroupStore) ListGroups(ctx context.Context) ([]identity.MemberGroup, error) {
	rows, err := store.client.QueryContext(ctx, `SELECT g.code,g.name,g.category,count(u.id)
        FROM identity_groups g LEFT JOIN user_identity_memberships m ON m.identity_code=g.code
        LEFT JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL
        GROUP BY g.code,g.name,g.category ORDER BY g.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]identity.MemberGroup, 0, 3)
	for rows.Next() {
		var group identity.MemberGroup
		if err := rows.Scan(&group.Code, &group.Name, &group.Category, &group.MemberCount); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (store *MemberGroupStore) Memberships(ctx context.Context, ids []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := clientFromContext(ctx, store.client).QueryContext(ctx,
		`SELECT user_id,identity_code FROM user_identity_memberships WHERE user_id=ANY($1) ORDER BY identity_code`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID int64
		var code string
		if err := rows.Scan(&userID, &code); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], code)
	}
	return result, rows.Err()
}

func (store *MemberGroupStore) RechargeIdentity(ctx context.Context, userID int64) (string, error) {
	client := clientFromContext(ctx, store.client)
	if dbent.TxFromContext(ctx) != nil {
		rows, err := client.QueryContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID)
		if err != nil {
			return "", err
		}
		exists := rows.Next()
		scanErr := rows.Err()
		rows.Close()
		if scanErr != nil {
			return "", scanErr
		}
		if !exists {
			return "", identity.ErrUserNotFound
		}
	}
	groups, err := store.Memberships(ctx, []int64{userID})
	if err != nil {
		return "", err
	}
	for _, code := range groups[userID] {
		if code == "bonus_5" || code == "bonus_10" {
			return code, nil
		}
	}
	return "", nil
}

func (store *MemberGroupStore) ChangeMembers(ctx context.Context, actorID int64, code string, ids []int64, add bool) error {
	category, err := identity.MemberGroupCategory(code)
	if err != nil {
		return err
	}
	tx, err := store.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	client := tx.Client()
	rows, err := client.QueryContext(ctx, `SELECT id FROM users WHERE id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(ids))
	if err != nil {
		return err
	}
	found := 0
	for rows.Next() {
		found++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if found != len(ids) {
		return identity.ErrUserNotFound
	}
	before, err := store.Memberships(dbent.NewTxContext(ctx, tx), ids)
	if err != nil {
		return err
	}
	if add {
		_, err = client.ExecContext(ctx, `INSERT INTO user_identity_memberships(user_id,category,identity_code)
            SELECT unnest($1::bigint[]),$2,$3 ON CONFLICT(user_id,category)
            DO UPDATE SET identity_code=EXCLUDED.identity_code,updated_at=now()`, pq.Array(ids), category, code)
	} else {
		_, err = client.ExecContext(ctx, `DELETE FROM user_identity_memberships WHERE user_id=ANY($1) AND identity_code=$2`, pq.Array(ids), code)
	}
	if err != nil {
		return err
	}
	detail, err := json.Marshal(map[string]any{"user_ids": ids, "before": before, "add": add})
	if err != nil {
		return err
	}
	_, err = client.ExecContext(ctx, `INSERT INTO identity_group_changes(actor_id,identity_code,action,details) VALUES($1,$2,'members',$3)`, actorID, code, string(detail))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (store *MemberGroupStore) ListMembers(ctx context.Context, code, search string, page, size int) ([]identity.GroupMember, int64, error) {
	filter := ` FROM users u WHERE u.deleted_at IS NULL
        AND ($1='' OR EXISTS(SELECT 1 FROM user_identity_memberships m WHERE m.user_id=u.id AND m.identity_code=$1))
        AND ($2='' OR u.username ILIKE '%'||$2||'%' OR u.email ILIKE '%'||$2||'%' OR u.id::text=$2)`
	rows, err := store.client.QueryContext(ctx, `SELECT count(*)`+filter, code, search)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if !rows.Next() {
		rows.Close()
		return nil, 0, fmt.Errorf("count identity members")
	}
	err = rows.Scan(&total)
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	rows, err = store.client.QueryContext(ctx, `SELECT u.id,u.username,u.email`+filter+` ORDER BY u.id LIMIT $3 OFFSET $4`, code, search, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	members := make([]identity.GroupMember, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		var member identity.GroupMember
		if err := rows.Scan(&member.UserID, &member.Username, &member.Email); err != nil {
			rows.Close()
			return nil, 0, err
		}
		members = append(members, member)
		ids = append(ids, member.UserID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	groups, err := store.Memberships(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for index := range members {
		members[index].IdentityGroups = groups[members[index].UserID]
	}
	return members, total, nil
}
