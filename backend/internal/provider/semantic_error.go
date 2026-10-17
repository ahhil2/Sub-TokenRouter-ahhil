package provider

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	SemanticErrorExtraKey = "semantic_error"
)

type SemanticErrorPolicy struct {
	Enabled         bool     `json:"enabled"`
	Texts           []string `json:"texts"`
	CooldownMinutes int      `json:"cooldown_minutes"`
}

func ValidateSemanticErrorExtra(extra map[string]any) error {
	raw, exists := extra[SemanticErrorExtraKey]
	if !exists || raw == nil {
		return nil
	}
	_, err := parseSemanticErrorPolicy(raw)
	if err != nil {
		return apperror.BadRequest("INVALID_SEMANTIC_ERROR_POLICY", "Response error rules require 1-10 texts of 8-512 characters and a cooldown of 1-1440 minutes")
	}
	return nil
}

func (record *Record) SemanticErrorPolicy() SemanticErrorPolicy {
	if record == nil || record.Extra[SemanticErrorExtraKey] == nil {
		return SemanticErrorPolicy{}
	}
	policy, err := parseSemanticErrorPolicy(record.Extra[SemanticErrorExtraKey])
	if err != nil {
		return SemanticErrorPolicy{}
	}
	return policy
}

func parseSemanticErrorPolicy(raw any) (SemanticErrorPolicy, error) {
	policy := SemanticErrorPolicy{}
	body, err := json.Marshal(raw)
	if err != nil {
		return policy, err
	}
	if err := json.Unmarshal(body, &policy); err != nil {
		return policy, err
	}
	if !policy.Enabled {
		return policy, nil
	}
	if len(policy.Texts) < 1 || len(policy.Texts) > 10 || policy.CooldownMinutes < 1 || policy.CooldownMinutes > 1440 {
		return policy, apperror.BadRequest("INVALID_SEMANTIC_ERROR_POLICY", "Invalid response error rule limits")
	}
	for index, text := range policy.Texts {
		text = strings.Join(strings.Fields(text), " ")
		if !utf8.ValidString(text) || utf8.RuneCountInString(text) < 8 || utf8.RuneCountInString(text) > 512 {
			return policy, apperror.BadRequest("INVALID_SEMANTIC_ERROR_POLICY", "Invalid response error text length")
		}
		policy.Texts[index] = text
	}
	return policy, nil
}
