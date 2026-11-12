package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/creative"
)

// prompt.go stores temporary expanded text and serializes matching requests.
// A request lease outlives the service's execution timeout, and cleanup compares
// its random ownership token before deleting it. This prevents delayed cleanup
// from deleting a newer owner's lease after an expiration or retry.
//
// Output records expire after thirty minutes to match Creative Studio's prompt
// privacy policy. The billing command is stored together with the text before
// settlement begins, so a request retry can recover an uncertain charge with
// the same financial identity. Durable billing receipts outlive these temporary
// records and prevent a paid request from being charged a second time.

// PromptStore retains the application-owned Redis client for prompt request state.
type PromptStore struct {
	client *redis.Client // shared caller-owned Redis connection
}

// NewPromptStore constructs prompt storage using a client owned by the application.
// The caller keeps the client usable through all requests and closes it after
// prompt operations and their lease cleanups have finished.
func NewPromptStore(client *redis.Client) *PromptStore { return &PromptStore{client: client} }

// Acquire creates a two-minute lease for one hashed request identity.
// The returned cleanup uses a separate bounded context because the request may
// already be cancelled when its deferred lease release runs. Concurrent callers
// receive acquired false while that key exists and do not own cleanup. A random
// token distinguishes replacement leases after expiration, so delayed cleanup
// deletes the key only when its stored token still belongs to this caller.
func (s *PromptStore) Acquire(ctx context.Context, id string) (func(), bool, error) {
	token, err := creative.NewCreativeRunID()
	if err != nil {
		return nil, false, err
	}
	key := "creative:prompt:lock:" + id
	acquired, err := s.client.SetNX(ctx, key, token, 2*time.Minute).Result()
	if err != nil || !acquired {
		return nil, acquired, err
	}
	return func() {
		// Ownership comparison keeps a timed-out request from clearing a replacement
		// lease. Failed cleanup leaves the key to its bounded expiration period.
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.client.Eval(cleanup, `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`, []string{key}, token).Err()
	}, true, nil
}

// Load returns a recovered output or nil when no temporary record exists.
// Invalid JSON and Redis errors propagate so an uncertain cache read cannot
// cause another upstream execution while a paid result might remain available.
func (s *PromptStore) Load(ctx context.Context, id string) (*creative.PromptExpansionRecord, error) {
	raw, err := s.client.Get(ctx, "creative:prompt:result:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result creative.PromptExpansionRecord
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Save retains expanded text and its financial snapshot for thirty minutes.
// Serialization finishes before the write, and a failed write prevents the
// caller from charging for an output it cannot recover after interruption.
func (s *PromptStore) Save(ctx context.Context, id string, record *creative.PromptExpansionRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "creative:prompt:result:"+id, raw, 30*time.Minute).Err()
}
