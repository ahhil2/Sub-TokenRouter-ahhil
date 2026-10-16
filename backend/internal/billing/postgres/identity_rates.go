package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const effectiveIdentityRatesSQL = `SELECT DISTINCT ON (group_id) group_id,rate_multiplier FROM (
    SELECT group_id,rate_multiplier,0 AS priority FROM user_group_rate_multipliers
    WHERE user_id=$1 AND rate_multiplier IS NOT NULL
    UNION ALL
    SELECT r.group_id,r.rate_multiplier,1 AS priority FROM identity_group_rates r
    JOIN user_identity_memberships m ON m.identity_code=r.identity_code WHERE m.user_id=$1
) rates`

func (store *GroupRateStore) EffectiveGroupRate(ctx context.Context, userID, groupID int64) (*float64, error) {
	rows, err := store.sql.QueryContext(ctx, effectiveIdentityRatesSQL+` WHERE group_id=$2 ORDER BY group_id,priority`, userID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
	defer rows.Close()
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
	defer rows.Close()
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
	defer tx.Rollback()
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
			rows.Close()
			return err
		}
		before[groupID] = value
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, rate := range rates {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND deleted_at IS NULL)`, rate.GroupID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.BadRequest("INVALID_IDENTITY_RATES", "group not found")
		}
		if rate.RateMultiplier == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=$1`, rate.GroupID)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO identity_group_rates(identity_code,group_id,rate_multiplier) VALUES('ahhilai',$1,$2)
                ON CONFLICT(identity_code,group_id) DO UPDATE SET rate_multiplier=EXCLUDED.rate_multiplier`, rate.GroupID, *rate.RateMultiplier)
		}
		if err != nil {
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
