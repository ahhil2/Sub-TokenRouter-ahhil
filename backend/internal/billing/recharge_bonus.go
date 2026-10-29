package billing

import (
	"context"
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// recharge_bonus.go calculates credited recharge amounts and selects the bonus
// snapshot used by redemption. A payment order captures identity at checkout;
// fulfillment can happen after membership changes, so payment redemption uses
// that saved snapshot. A typed nil snapshot identifies a historical payment
// whose purchased amount is credited directly. Ordinary code redemption reads
// current membership when its positive amount is being redeemed.
//
// Decimal arithmetic rounds purchased and gifted amounts to eight decimal
// places before saving the credited total. Refunds recover credit in proportion
// to the stored purchased amount, including its gift. The selected snapshot is
// copied before returning it to redemption, and a mismatched payment amount
// stops credit rather than combining terms from different purchases.

// RechargeSnapshot records purchased credit and the identity bonus applied to it.
// BaseAmount plus BonusAmount equals CreditedAmount; IdentityCode selects the
// BonusRate used when those amounts were calculated, so fulfillment can use the
// saved terms after membership changes.
type RechargeSnapshot struct {
	IdentityCode   string  `json:"identity_code"`   // checkout or redemption identity
	BaseAmount     float64 `json:"base_amount"`     // purchased reasoning point amount
	BonusRate      float64 `json:"bonus_rate"`      // fractional identity bonus rate
	BonusAmount    float64 `json:"bonus_amount"`    // additional reasoning point amount
	CreditedAmount float64 `json:"credited_amount"` // purchased plus gifted points
}

// paymentRechargeKey identifies payment-owned snapshot values in redemption contexts.
type paymentRechargeKey struct{}

// CalculateRecharge returns purchased and gifted amounts for a finite base and
// a recognized identity code. Unknown identities and nonfinite amounts return
// errors, while nonpositive bases clear identity and disable gifts.
//
// The purchased amount is rounded to eight decimal places before calculating
// the percentage gift, which is rounded separately. The credited total adds
// those stored quantities so later fulfillment and proportional refunds can use
// one set of saved terms instead of rereading membership.
func CalculateRecharge(base float64, identityCode string) (*RechargeSnapshot, error) {
	if math.IsNaN(base) || math.IsInf(base, 0) {
		return nil, fmt.Errorf("invalid recharge amount")
	}
	rate := int64(0)
	switch identityCode {
	case "bonus_5":
		rate = 5
	case "bonus_10":
		rate = 10
	case "":
	default:
		return nil, fmt.Errorf("invalid recharge identity")
	}
	if base <= 0 {
		rate = 0
		identityCode = ""
	}
	// Round the base before the gift so saved purchased and credited quantities
	// remain consistent when a later refund calculates their proportional ratio.
	amount := decimal.NewFromFloat(base).Round(8)
	bonus := amount.Mul(decimal.NewFromInt(rate)).Div(decimal.NewFromInt(100)).Round(8)
	return &RechargeSnapshot{
		IdentityCode:   identityCode,
		BaseAmount:     amount.InexactFloat64(),
		BonusRate:      float64(rate) / 100,
		BonusAmount:    bonus.InexactFloat64(),
		CreditedAmount: amount.Add(bonus).InexactFloat64(),
	}, nil
}

// RefundCredit converts a finite purchased-credit refund into proportional wallet
// recovery, rounded to eight decimal places. A nil snapshot or nonpositive saved
// base returns the supplied refund amount for historical orders.
func (snapshot *RechargeSnapshot) RefundCredit(baseRefund float64) float64 {
	if snapshot == nil || snapshot.BaseAmount <= 0 {
		return baseRefund
	}
	// Recover gifted credit using the stored ratio, keeping refunds at the
	// same eight-decimal precision used when the recharge snapshot was calculated.
	return decimal.NewFromFloat(baseRefund).
		Mul(decimal.NewFromFloat(snapshot.CreditedAmount)).
		Div(decimal.NewFromFloat(snapshot.BaseAmount)).Round(8).InexactFloat64()
}

// ContextPaymentRecharge marks redemption as payment-owned using a saved snapshot.
// Supply a nonnil context and keep a nonnil snapshot stable until redemption;
// a typed nil snapshot selects historical payment behavior.
func ContextPaymentRecharge(ctx context.Context, snapshot *RechargeSnapshot) context.Context {
	return context.WithValue(ctx, paymentRechargeKey{}, snapshot)
}

// rechargeSnapshot selects saved payment terms or calculates current redemption
// terms for a nonnil code. A saved payment snapshot is copied and its base must
// match the code value before credit can proceed.
//
// A present typed nil payment value selects an ordinary historical recharge;
// absence of the payment marker permits a current identity lookup for positive
// code values. Identity lookup errors stop redemption, preventing an unavailable
// membership read from silently producing a different credited amount.
func (s *RedeemService) rechargeSnapshot(ctx context.Context, userID int64, code *RedeemCode) (*RechargeSnapshot, error) {
	if snapshot, present := ctx.Value(paymentRechargeKey{}).(*RechargeSnapshot); present {
		if snapshot != nil {
			copy := *snapshot
			if copy.BaseAmount != code.Value {
				return nil, fmt.Errorf("payment recharge snapshot amount mismatch")
			}
			return &copy, nil
		}
		return CalculateRecharge(code.Value, "")
	}
	identityCode := ""
	if code.Value > 0 && s.runtime.RechargeIdentity != nil {
		var err error
		identityCode, err = s.runtime.RechargeIdentity(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	return CalculateRecharge(code.Value, identityCode)
}
