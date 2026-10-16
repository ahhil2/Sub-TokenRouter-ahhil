package billing

import (
	"context"
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

type RechargeSnapshot struct {
	IdentityCode   string  `json:"identity_code"`
	BaseAmount     float64 `json:"base_amount"`
	BonusRate      float64 `json:"bonus_rate"`
	BonusAmount    float64 `json:"bonus_amount"`
	CreditedAmount float64 `json:"credited_amount"`
}

type paymentRechargeKey struct{}

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

func (snapshot *RechargeSnapshot) RefundCredit(baseRefund float64) float64 {
	if snapshot == nil || snapshot.BaseAmount <= 0 {
		return baseRefund
	}
	return decimal.NewFromFloat(baseRefund).
		Mul(decimal.NewFromFloat(snapshot.CreditedAmount)).
		Div(decimal.NewFromFloat(snapshot.BaseAmount)).Round(8).InexactFloat64()
}

func ContextPaymentRecharge(ctx context.Context, snapshot *RechargeSnapshot) context.Context {
	return context.WithValue(ctx, paymentRechargeKey{}, snapshot)
}

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
