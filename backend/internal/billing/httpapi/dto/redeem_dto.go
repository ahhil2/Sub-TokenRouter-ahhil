package dto

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// redeem_dto.go defines redemption responses and decodes presence-aware expiry
// updates.
// Redeem responses expose entitlement amounts and the most recent successful
// use, with ordinary users receiving notes for administrator adjustments.
// Administrator responses add the full internal notes field. Batch expiry
// updates also need to distinguish an omitted value from clearing an expiry
// with JSON null.
//
// NullableTimeField retains field presence separately from the timestamp
// pointer. UnmarshalJSON marks presence before decoding and uses a nil pointer
// for explicit null; invalid timestamp input returns an error after that
// presence change. The handler decodes a fresh request and checks Set when
// building the domain update, so omitted fields leave saved expiry in place.

// RedeemCode is the user response for a redemption or administrator adjustment
// record.
type RedeemCode struct {
	RechargeSnapshot *billing.RechargeSnapshot `json:"recharge_snapshot,omitempty"`
	RequiresPayment  bool                      `json:"requires_payment"` // 领取者需要有成功付款记录
	ID               int64                     `json:"id"`
	Code             string                    `json:"code"`
	Type             string                    `json:"type"`
	Value            float64                   `json:"value"`
	Status           string                    `json:"status"`
	MaxUses          int                       `json:"max_uses"`
	UsedCount        int                       `json:"used_count"`
	ExpiresAt        *time.Time                `json:"expires_at"`
	UsedBy           *int64                    `json:"used_by"` // 最后一次成功兑换的用户
	UsedAt           *time.Time                `json:"used_at"` // 最后一次成功兑换的时间
	CreatedAt        time.Time                 `json:"created_at"`

	// PlanID selects the plan for subscription entitlement codes. The response
	// mapper retains this optional pointer from the service record for read-only
	// consumption.
	PlanID *int64 `json:"plan_id"` // subscription grant plan identifier

	// Notes references the service record's reason for nonempty administrator
	// balance or concurrency adjustments. Callers keep this shared string
	// unchanged; other types omit internal notes from the user response.
	Notes *string `json:"notes,omitempty"` // visible administrator adjustment reason

	User *UserSummaryResponse `json:"user,omitempty"` // hydrated latest recipient details
}

// AdminRedeemCode adds unrestricted administrator notes to the user redemption
// response fields.
type AdminRedeemCode struct {
	RedeemCode // common user redemption response

	Notes string `json:"notes"` // full administrator notes text
}

// NullableTimeField represents JSON expiry presence separately from its
// optional timestamp value.
type NullableTimeField struct {
	// Set and Value distinguish omission, explicit null and a timestamp in a
	// fresh or reset field. After successful decoding, Set true with nil Value
	// requests an expiry clear; omission retains the field's prior state.
	Set   bool       // expiry field was supplied
	Value *time.Time // decoded optional expiry timestamp
}

// BatchUpdateRedeemCodeFields contains supplied batch changes and core-field
// attempts for service validation.
type BatchUpdateRedeemCodeFields struct {
	Status *string `json:"status,omitempty"` // optional status replacement value
	// ExpiresAt carries field presence to the batch handler. Set false preserves
	// the stored expiry, while Set true and nil Value requests clearing it.
	ExpiresAt NullableTimeField `json:"expires_at,omitempty"` // supplied expiry field update
	Notes     *string           `json:"notes,omitempty"`      // optional notes replacement text

	// Type, Value, MaxUses and PlanID retain attempted entitlement changes for
	// validation. RedeemService rejects these batch core changes and requires the
	// single-code consistency checks.
	Type    *string  `json:"type,omitempty"`     // attempted entitlement type change
	Value   *float64 `json:"value,omitempty"`    // attempted entitlement amount change
	MaxUses *int     `json:"max_uses,omitempty"` // attempted maximum redemption change
	PlanID  *int64   `json:"plan_id,omitempty"`  // attempted subscription plan change
}

// BatchUpdateRedeemCodesRequest supplies target code IDs and their requested
// common field changes.
type BatchUpdateRedeemCodesRequest struct {
	IDs    []int64                     `json:"ids" binding:"required,min=1"` // requested target code identifiers
	Fields BatchUpdateRedeemCodeFields `json:"fields" binding:"required"`    // common requested field changes
}

// UnmarshalJSON records a supplied expiry field and decodes a timestamp or
// null. Whitespace around null is accepted, and null clears Value while Set
// remains true, allowing the handler to pass an intentional clear to the
// domain update.
//
// Omission never calls this method and retains prior field state. Callers
// decode into a fresh or reset field when omission must leave Set false. Set
// becomes true before timestamp parsing, and a previous Value can remain after
// a parsing error; callers discard the failed request before applying changes.
func (f *NullableTimeField) UnmarshalJSON(data []byte) error {
	// Record presence before examining the payload so explicit null survives
	// decoding as an intentional clear rather than an omitted expiry.
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Value = &value
	return nil
}
