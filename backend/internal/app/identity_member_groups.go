package app

import (
	"context"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingpostgres "github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	identityhttp "github.com/TokenFlux/TokenRouter/internal/identity/httpapi"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
)

func provideMemberGroupHandler(client *dbent.Client, rates *billingpostgres.GroupRateStore, auth apikey.APIKeyAuthCacheInvalidator) *identityhttp.MemberGroupHandler {
	groups := &identity.MemberGroups{
		Repository: identitypostgres.NewMemberGroupStore(client),
		Invalidate: auth.InvalidateAuthCacheByUserID,
	}
	return &identityhttp.MemberGroupHandler{
		Groups: groups,
		Rates:  rates,
		InvalidateRates: func(ctx context.Context, inputs []billing.IdentityRateInput) {
			for _, input := range inputs {
				auth.InvalidateAuthCacheByGroupID(ctx, input.GroupID)
			}
		},
	}
}
