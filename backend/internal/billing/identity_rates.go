package billing

import "context"

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
