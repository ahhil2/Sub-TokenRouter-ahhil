//go:build integration

package payment_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/paymentauditlog"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymentpostgres "github.com/TokenFlux/TokenRouter/internal/payment/postgres"
	"github.com/TokenFlux/TokenRouter/internal/promotion"
	promotionpostgres "github.com/TokenFlux/TokenRouter/internal/promotion/postgres"
	"github.com/TokenFlux/TokenRouter/internal/settings"
	settingspostgres "github.com/TokenFlux/TokenRouter/internal/settings/postgres"
)

// checkout_identity_integration_test.go checks recharge snapshots and order-linked
// affiliate grants against PostgreSQL. Checkout reads the buyer's current recharge
// identity and persists the resulting gift with the order. Changing that identity
// afterward must leave the saved purchased and credited amounts intact.
//
// Rebate accrual consumes the reloaded order's purchased-credit base through the
// payment store's transaction and the promotion repository. The case checks both
// the inviter's quota and the source-order ledger, then repeats the order grant to
// exercise its persisted audit claim. Shared affiliate settings are restored after
// the test so subsequent refund and payment cases retain their initial policy.

// TestCheckoutIdentitySnapshotAndRebatePostgres changes recharge identity after
// saving an order with a five-percent gift. Reloaded snapshots keep that gift,
// and a replayed ten-percent affiliate grant credits the purchased base once.
func TestCheckoutIdentitySnapshotAndRebatePostgres(t *testing.T) {
	ctx := t.Context()
	client := testEntClient(t)
	buyer, err := client.User.Create().SetEmail("checkout-buyer@example.invalid").
		SetPasswordHash("fixture").SetUsername("buyer").Save(ctx)
	require.NoError(t, err)
	inviter, err := client.User.Create().SetEmail("checkout-inviter@example.invalid").
		SetPasswordHash("fixture").SetUsername("inviter").Save(ctx)
	require.NoError(t, err)
	members := identitypostgres.NewMemberGroupStore(client)
	require.NoError(t, members.ChangeMembers(ctx, inviter.ID, "bonus_5", []int64{buyer.ID}, true))
	affiliates := promotionpostgres.NewAffiliateRepository(client, nil)
	_, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = affiliates.EnsureUserAffiliate(ctx, buyer.ID)
	require.NoError(t, err)
	_, err = affiliates.BindInviter(ctx, buyer.ID, inviter.ID)
	require.NoError(t, err)

	store := settings.New(settingspostgres.NewSettingRepository(client))
	keys := []string{
		promotion.SettingKeyAffiliateEnabled, promotion.SettingKeyAffiliateRebateRate,
		promotion.SettingKeyAffiliateRebatePerInviteeCap, promotion.SettingKeyAffiliateRebateFreezeHours,
		promotion.SettingKeyAffiliateRebateDurationDays,
	}
	original, err := store.GetMultiple(ctx, keys)
	require.NoError(t, err)
	// Remove temporary keys before restoring saved values so originally absent
	// settings do not leak this test's policy into later package cases.
	t.Cleanup(func() {
		for _, key := range keys {
			require.NoError(t, store.Delete(context.Background(), key))
		}
		require.NoError(t, store.SetMultiple(context.Background(), original))
	})
	require.NoError(t, store.SetMultiple(ctx, map[string]string{
		promotion.SettingKeyAffiliateEnabled:             "true",
		promotion.SettingKeyAffiliateRebateRate:          "10",
		promotion.SettingKeyAffiliateRebatePerInviteeCap: "0",
		promotion.SettingKeyAffiliateRebateFreezeHours:   "0",
		promotion.SettingKeyAffiliateRebateDurationDays:  "0",
	}))
	policy := promotion.NewRuntimeSettings(store)
	orders := paymentpostgres.NewOrderStore(client, paymentpostgres.OrderStoreRuntime{
		Rebates: func(tx *ent.Tx) paymentpostgres.OrderRebates {
			return promotion.NewAffiliateService(promotionpostgres.NewAffiliateRepository(tx.Client(), nil),
				policy, nil, nil, promotion.Runtime{})
		},
	})
	checkout := payment.NewCheckout(orders, nil, nil, nil, payment.CheckoutRuntime{
		RechargeIdentity: members.RechargeIdentity,
	})
	order, err := checkout.CreateOrderInTx(ctx,
		payment.CreateOrderRequest{UserID: buyer.ID, OrderType: payment.OrderTypeBalance, PaymentType: payment.TypeStripe},
		&payment.Buyer{ID: buyer.ID, Email: buyer.Email, Username: buyer.Username}, nil,
		&payment.PaymentConfig{}, 100, 100, payment.FeeBreakdown{}, nil)
	require.NoError(t, err)
	// Delete order audits before the order itself, while the shared fixture's
	// later user truncation removes the buyer, inviter, and their dependent data.
	t.Cleanup(func() {
		_, cleanupErr := client.PaymentAuditLog.Delete().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10))).Exec(context.Background())
		require.NoError(t, cleanupErr)
		require.NoError(t, client.PaymentOrder.DeleteOneID(order.ID).Exec(context.Background()))
	})
	require.Equal(t, 105.0, order.RechargeSnapshot.CreditedAmount)
	require.NoError(t, members.ChangeMembers(ctx, inviter.ID, "bonus_10", []int64{buyer.ID}, true))
	saved, err := orders.Order(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "bonus_5", saved.RechargeSnapshot.IdentityCode)
	require.Equal(t, 105.0, saved.RechargeSnapshot.CreditedAmount)
	require.Equal(t, 100.0, payment.AffiliateRebateBasePoints(saved))
	require.NoError(t, client.PaymentOrder.UpdateOneID(order.ID).SetStatus(payment.OrderStatusCompleted).Exec(ctx))

	// The second grant uses the same persisted order claim, so a repeated
	// fulfillment request must not increase either promotion quota or ledger rows.
	for range 2 {
		require.NoError(t, orders.ApplyOrderRebate(ctx, saved, payment.AffiliateRebateBasePoints(saved)))
	}
	var quota, ledgerAmount float64
	var ledgerCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT aff_quota FROM user_affiliates WHERE user_id=$1", inviter.ID).Scan(&quota))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*), COALESCE(sum(amount),0) FROM user_affiliate_ledger WHERE source_order_id=$1", order.ID).
		Scan(&ledgerCount, &ledgerAmount))
	require.Equal(t, 10.0, quota)
	require.Equal(t, 10.0, ledgerAmount)
	require.Equal(t, 1, ledgerCount)
}
