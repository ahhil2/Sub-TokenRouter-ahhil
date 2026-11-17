package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// identity_rates.go reads and edits identity-based balance multipliers. It resolves the
// effective multiplier a user pays for a group, lists every group with its AHHilAI
// override, and replaces AHHilAI overrides in bulk for the admin settings page.
//
// The effective rate takes the first available source: a personal override, then an
// AHHilAI group override, then the group default discounted for bonus_5 and bonus_10
// members when identity_discount_exempt is false (factors come from
// billing.IdentityRateDiscountSQL). A user with none of these
// has no effective multiplier, and callers use the undiscounted group default. Bulk edits
// run in one transaction under a lock on the identity row, record the previous overrides
// in the change audit, and use set-based statements so the number of round trips does not
// grow with the number of groups. A missing group rolls back the whole edit, so neither
// the overrides nor the audit trail change.

// effectiveIdentityRatesSQL selects per-group candidate multipliers with their
// priority; callers append a filter and ORDER BY group_id,priority for DISTINCT ON.
var effectiveIdentityRatesSQL = buildEffectiveIdentityRatesSQL()

// buildEffectiveIdentityRatesSQL builds candidate balance rates in descending
// precedence: personal overrides, AHHilAI overrides, and identity discounts.
// Discount factors come from the billing table, and original-price groups exclude
// only the discount candidates. Their callers then use the group's default rate
// when no explicit override exists. A configured zero remains a candidate, so
// disabling membership discounts cannot replace a free personal or AHHilAI rate.
// @project-doc docs/domains/user_identity_groups.md#pricing
func buildEffectiveIdentityRatesSQL() string {
	codes, factor := billing.IdentityRateDiscountSQL("m.identity_code")
	return `SELECT DISTINCT ON (group_id) group_id,rate_multiplier FROM (
    SELECT group_id,rate_multiplier,0 AS priority FROM user_group_rate_multipliers
    WHERE user_id=$1 AND rate_multiplier IS NOT NULL
    UNION ALL
    SELECT r.group_id,r.rate_multiplier,1 AS priority FROM identity_group_rates r
    JOIN user_identity_memberships m ON m.identity_code=r.identity_code WHERE m.user_id=$1
    UNION ALL
    SELECT g.id,g.rate_multiplier*` + factor + `,2 AS priority FROM groups g
    JOIN user_identity_memberships m ON m.user_id=$1 AND m.identity_code IN (` + codes + `)
    WHERE g.deleted_at IS NULL AND NOT g.identity_discount_exempt
) rates`
}

// EffectiveGroupRate returns the multiplier userID pays for groupID, or nil when no
// personal override, AHHilAI override or discount identity applies. Earlier sources take
// precedence, and a configured zero multiplier is returned as zero rather than nil.
func (store *GroupRateStore) EffectiveGroupRate(ctx context.Context, userID, groupID int64) (*float64, error) {
	rows, err := store.sql.QueryContext(ctx, effectiveIdentityRatesSQL+` WHERE group_id=$2 ORDER BY group_id,priority`, userID, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var rate float64
	var id int64
	if err := rows.Scan(&id, &rate); err != nil {
		return nil, err
	}
	return &rate, rows.Err()
}

func (store *GroupRateStore) EffectiveGroupRates(ctx context.Context, userID int64) (map[int64]float64, error) {
	rows, err := store.sql.QueryContext(ctx, effectiveIdentityRatesSQL+` ORDER BY group_id,priority`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	rates := make(map[int64]float64)
	for rows.Next() {
		var id int64
		var rate float64
		if err := rows.Scan(&id, &rate); err != nil {
			return nil, err
		}
		rates[id] = rate
	}
	return rates, rows.Err()
}

func (store *GroupRateStore) IdentityRates(ctx context.Context) ([]billing.IdentityRate, error) {
	rows, err := store.sql.QueryContext(ctx, `SELECT g.id,g.name,g.rate_multiplier,r.rate_multiplier
        FROM groups g LEFT JOIN identity_group_rates r ON r.group_id=g.id AND r.identity_code='ahhilai'
        WHERE g.deleted_at IS NULL ORDER BY g.sort_order,g.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	rates := make([]billing.IdentityRate, 0)
	for rows.Next() {
		var rate billing.IdentityRate
		var value sql.NullFloat64
		if err := rows.Scan(&rate.GroupID, &rate.Name, &rate.DefaultMultiplier, &value); err != nil {
			return nil, err
		}
		if value.Valid {
			rate.RateMultiplier = &value.Float64
		}
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

// UpdateIdentityRates applies the supplied ahhilai multipliers and records the change
// under actorID. An input with a nil multiplier removes that group's override, and any
// other input sets or replaces it. Groups not listed keep their current overrides.
//
// Inputs are rejected before any write when there are more than 1000, when a group ID
// is not positive or repeats, or when a multiplier is negative, not finite, or at least
// 1e12. A group that does not exist or is deleted returns INVALID_IDENTITY_RATES.
//
// The identity row is locked first so concurrent edits serialize, and the previous
// overrides are captured for the audit entry before anything changes. Existence is
// checked once for all groups, then deletes and upserts run as one statement each. Any
// failure rolls back the overrides and the audit entry together.
func (store *GroupRateStore) UpdateIdentityRates(ctx context.Context, actorID int64, rates []billing.IdentityRateInput) error {
	if len(rates) > 1000 {
		return apperror.BadRequest("INVALID_IDENTITY_RATES", "too many group rates")
	}
	seen := make(map[int64]bool)
	for _, rate := range rates {
		if rate.GroupID <= 0 || seen[rate.GroupID] {
			return apperror.BadRequest("INVALID_IDENTITY_RATES", "invalid or duplicate group")
		}
		seen[rate.GroupID] = true
		if rate.RateMultiplier != nil {
			value := *rate.RateMultiplier
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value >= 1e12 {
				return apperror.BadRequest("INVALID_IDENTITY_RATES", "invalid rate multiplier")
			}
		}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT code FROM identity_groups WHERE code='ahhilai' FOR UPDATE`); err != nil {
		return err
	}
	before := make(map[int64]float64)
	rows, err := tx.QueryContext(ctx, `SELECT group_id,rate_multiplier FROM identity_group_rates WHERE identity_code='ahhilai'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var groupID int64
		var value float64
		if err := rows.Scan(&groupID, &value); err != nil {
			_ = rows.Close()
			return err
		}
		before[groupID] = value
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	// Split the validated input once so each write below is a single set-based statement
	// whose round trips stay constant as the group count grows. Group IDs are unique
	// after validation.
	groupIDs := make([]int64, 0, len(rates))
	clearIDs := make([]int64, 0, len(rates))
	setIDs := make([]int64, 0, len(rates))
	setValues := make([]float64, 0, len(rates))
	for _, rate := range rates {
		groupIDs = append(groupIDs, rate.GroupID)
		if rate.RateMultiplier == nil {
			clearIDs = append(clearIDs, rate.GroupID)
			continue
		}
		setIDs = append(setIDs, rate.GroupID)
		setValues = append(setValues, *rate.RateMultiplier)
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE id=ANY($1) AND deleted_at IS NULL`, pq.Array(groupIDs)).Scan(&existing); err != nil {
		return err
	}
	if existing != len(groupIDs) {
		return apperror.BadRequest("INVALID_IDENTITY_RATES", "group not found")
	}
	if len(clearIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=ANY($1)`, pq.Array(clearIDs)); err != nil {
			return err
		}
	}
	if len(setIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO identity_group_rates(identity_code,group_id,rate_multiplier)
                SELECT 'ahhilai', input.group_id, input.rate FROM unnest($1::bigint[], $2::float8[]) AS input(group_id, rate)
                ON CONFLICT(identity_code,group_id) DO UPDATE SET rate_multiplier=EXCLUDED.rate_multiplier`, pq.Array(setIDs), pq.Array(setValues)); err != nil {
			return err
		}
	}
	detail, err := json.Marshal(map[string]any{"before": before, "changes": rates})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO identity_group_changes(actor_id,identity_code,action,details) VALUES($1,'ahhilai','rates',$2)`, actorID, string(detail)); err != nil {
		return err
	}
	return tx.Commit()
}
