//go:build integration

package payment_test

import (
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent/paymentauditlog"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/payment"
)

// refund_identity_integration_test.go checks saved recharge gifts through the
// payment refund workflow and PostgreSQL balance transactions. Refund amounts
// describe purchased credit, while the wallet deduction includes the gift in
// the order's recharge snapshot. Local Stripe responses exercise immediate and
// pending refunds through the same storage implementation used by the service.
//
// A pending provider response compensates the initial wallet deduction before
// a later confirmation reclaims the available balance. These cases inspect
// persisted balances, snapshot values, order flags, and success audits after
// each transition. Repeated confirmation must not deduct again after settlement,
// including when spending reduced the payer's balance before provider confirmation.

// TestIdentityPartialRefundPostgres refunds one quarter of orders with five
// and ten percent gifts. Persisted deductions include proportional gifts while
// the payment provider receives the purchased-credit refund amount.
func TestIdentityPartialRefundPostgres(t *testing.T) {
	for _, identityCode := range []string{"bonus_5", "bonus_10"} {
		t.Run(identityCode, func(t *testing.T) {
			ctx := t.Context()
			client, workflow, order := refundOrderFixture(t, payment.OrderStatusCompleted)
			snapshot, err := billing.CalculateRecharge(100, identityCode)
			require.NoError(t, err)
			require.NoError(t, client.PaymentOrder.UpdateOneID(order.ID).
				SetAmount(100).SetPayAmount(100).SetRechargeSnapshot(snapshot).Exec(ctx))
			require.NoError(t, client.User.UpdateOneID(order.UserID).
				SetBalance(snapshot.CreditedAmount).Exec(ctx))

			var submissions atomic.Int32
			// Response-write errors are ignored by this local fixture because the
			// workflow result and persisted balance assertions detect unsuccessful exchanges.
			refundStripeFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				submissions.Add(1)
				if err := request.ParseForm(); err != nil {
					t.Errorf("parse provider request: %v", err)
				}
				// Stripe expresses USD in cents, excluding gifted credit.
				if got := request.FormValue("amount"); got != "2500" {
					t.Errorf("provider refund cents = %q, want 2500", got)
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"id":"re_identity","object":"refund","status":"succeeded"}`))
			})

			plan, warning, err := workflow.PrepareRefund(ctx, order.ID, 25, "partial gift refund", false, true)
			require.NoError(t, err)
			require.Nil(t, warning)
			// Quarter-order refunds reclaim proportional purchased and gifted credit.
			expectedDeduction := 26.25
			if identityCode == "bonus_10" {
				expectedDeduction = 27.5
			}
			require.Equal(t, expectedDeduction, plan.BalanceToDeduct)
			result, err := workflow.ExecuteRefund(ctx, plan)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, expectedDeduction, result.BalanceDeducted)
			require.EqualValues(t, 1, submissions.Load())
			assertRefundPostgresState(t, ctx, client, order.UserID, order.ID,
				snapshot.CreditedAmount-expectedDeduction, payment.OrderStatusPartiallyRefunded, 1)
			saved, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, snapshot, saved.RechargeSnapshot)
			require.Equal(t, 25.0, saved.RefundAmount)
			require.False(t, saved.ForceRefund)
		})
	}
}

// TestIdentityPendingRefundPostgresClampsAvailableBalance spends the restored
// wallet while a gifted recharge refund awaits provider confirmation. Final
// settlement deducts the remaining balance once and retains the nonforced flag.
func TestIdentityPendingRefundPostgresClampsAvailableBalance(t *testing.T) {
	ctx := t.Context()
	client, workflow, order := refundOrderFixture(t, payment.OrderStatusCompleted)
	snapshot, err := billing.CalculateRecharge(100, "bonus_10")
	require.NoError(t, err)
	require.NoError(t, client.PaymentOrder.UpdateOneID(order.ID).
		SetAmount(100).SetPayAmount(100).SetRechargeSnapshot(snapshot).Exec(ctx))
	require.NoError(t, client.User.UpdateOneID(order.UserID).SetBalance(110).Exec(ctx))
	var submissions atomic.Int32
	// The fixture ignores response-write errors for both provider observations;
	// pending compensation and final deduction assertions independently require
	// successful exchanges.
	refundStripeFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			submissions.Add(1)
			_, _ = writer.Write([]byte(`{"id":"re_pending_identity","object":"refund","status":"pending"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"re_pending_identity","object":"refund","status":"succeeded"}`))
	})
	plan, warning, err := workflow.PrepareRefund(ctx, order.ID, 100, "pending gift refund", false, true)
	require.NoError(t, err)
	require.Nil(t, warning)
	require.Equal(t, 110.0, plan.BalanceToDeduct)
	result, err := workflow.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	assertRefundPostgresState(t, ctx, client, order.UserID, order.ID, 110, payment.OrderStatusRefundPending, 0)

	// Spending after pending compensation reproduces a lower wallet balance at
	// confirmation, when the provider's successful refund can no longer be rejected.
	require.NoError(t, client.User.UpdateOneID(order.UserID).SetBalance(7.25).Exec(ctx))
	result, err = workflow.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 7.25, result.BalanceDeducted)
	assertRefundPostgresState(t, ctx, client, order.UserID, order.ID, 0, payment.OrderStatusRefunded, 1)
	saved, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, saved.ForceRefund)
	audit, err := client.PaymentAuditLog.Query().Where(
		paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
		paymentauditlog.ActionEQ("REFUND_SUCCESS"),
	).Only(ctx)
	require.NoError(t, err)
	require.Contains(t, audit.Detail, `"balanceDeducted":7.25`)
	require.Contains(t, audit.Detail, `"force":false`)

	// Replay observes the terminal order state before another provider submission
	// or balance write, retaining the first successful deduction and its audit.
	_, err = workflow.QueryAndFinalizeRefund(ctx, order.ID)
	require.Error(t, err)
	require.EqualValues(t, 1, submissions.Load())
	assertRefundPostgresState(t, ctx, client, order.UserID, order.ID, 0, payment.OrderStatusRefunded, 1)
}
