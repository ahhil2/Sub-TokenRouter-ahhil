package billing

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// identity_rates.go defines identity-based balance pricing: AHHilAI group overrides and
// the bonus_5 and bonus_10 discounts. Balance charges resolve a user's multiplier from a
// personal override first, then an AHHilAI group override, and otherwise from the group
// default. Unless the group enables original pricing, members of bonus_5 and bonus_10
// pay 95% and 90% of that group default. The discount never applies on top of a
// personal or AHHilAI override, and subscription
// charges keep their own plan and group rates.
//
// The discount table is the single source for these factors. The PostgreSQL effective
// rate query embeds it through IdentityRateDiscountSQL, so every balance path that reads
// the effective rate (gateway settlement and admission, creative, batch image and price
// displays) applies the same discount.

// identityRateDiscounts maps discount identity codes to the factor applied to a
// group's default balance multiplier for their members.
var identityRateDiscounts = map[string]float64{
	"bonus_5":  0.95,
	"bonus_10": 0.9,
}

// IdentityRate describes a group's default multiplier and its AHHilAI override.
type IdentityRate struct {
	GroupID           int64    `json:"group_id"`
	Name              string   `json:"name"`
	DefaultMultiplier float64  `json:"default_multiplier"`
	RateMultiplier    *float64 `json:"rate_multiplier"`
}

type IdentityRateInput struct {
	GroupID        int64    `json:"group_id"`
	RateMultiplier *float64 `json:"rate_multiplier"`
}

type EffectiveRateReader interface {
	EffectiveGroupRate(context.Context, int64, int64) (*float64, error)
	EffectiveGroupRates(context.Context, int64) (map[int64]float64, error)
}

func EffectiveGroupRate(ctx context.Context, repo interface {
	GetByUserAndGroup(context.Context, int64, int64) (*float64, error)
}, userID, groupID int64,
) (*float64, error) {
	if reader, ok := repo.(EffectiveRateReader); ok {
		return reader.EffectiveGroupRate(ctx, userID, groupID)
	}
	if repo == nil {
		return nil, nil
	}
	return repo.GetByUserAndGroup(ctx, userID, groupID)
}

func EffectiveGroupRates(ctx context.Context, repo interface {
	GetByUserID(context.Context, int64) (map[int64]float64, error)
}, userID int64,
) (map[int64]float64, error) {
	if reader, ok := repo.(EffectiveRateReader); ok {
		return reader.EffectiveGroupRates(ctx, userID)
	}
	if repo == nil {
		return nil, nil
	}
	return repo.GetByUserID(ctx, userID)
}

func (resolver *GroupRateResolver) ResolveChecked(ctx context.Context, userID, groupID int64, fallback float64) (float64, error) {
	if resolver == nil {
		return fallback, nil
	}
	if _, ok := resolver.repo.(EffectiveRateReader); !ok {
		return resolver.Resolve(ctx, userID, groupID, fallback), nil
	}
	rate, err := EffectiveGroupRate(ctx, resolver.repo, userID, groupID)
	if err != nil {
		return 0, err
	}
	if rate != nil {
		return *rate, nil
	}
	return fallback, nil
}

// IdentityRateDiscount returns the balance discount factor for an identity code, or 1
// when the code grants no discount.
func IdentityRateDiscount(code string) float64 {
	if factor, ok := identityRateDiscounts[code]; ok {
		return factor
	}
	return 1
}

// IdentityRateDiscountSQL returns SQL fragments for the discount identities: a quoted
// code list for an IN clause, and a CASE expression mapping codeColumn to its factor.
// codeColumn is trusted SQL text supplied by the caller. Codes are emitted in sorted
// order so the generated query text is stable.
func IdentityRateDiscountSQL(codeColumn string) (codes, factor string) {
	names := make([]string, 0, len(identityRateDiscounts))
	for code := range identityRateDiscounts {
		names = append(names, code)
	}
	slices.Sort(names)
	quoted := make([]string, len(names))
	var cases strings.Builder
	_, _ = fmt.Fprintf(&cases, "CASE %s", codeColumn)
	for index, code := range names {
		quoted[index] = "'" + code + "'"
		_, _ = fmt.Fprintf(&cases, " WHEN '%s' THEN %s", code, strconv.FormatFloat(identityRateDiscounts[code], 'f', -1, 64))
	}
	_, _ = cases.WriteString(" ELSE 1 END")
	return strings.Join(quoted, ","), cases.String()
}
