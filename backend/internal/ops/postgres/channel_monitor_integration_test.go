//go:build integration

package postgres

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorSQLVisibilityAndMetrics(t *testing.T) {
	ctx := t.Context()
	transaction, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = transaction.Rollback() }()
	_, err = transaction.ExecContext(ctx, `
CREATE TEMP TABLE groups (id bigint, name text, display_brand text, rate_multiplier float8, sort_order int DEFAULT 0, status text DEFAULT 'active', is_exclusive bool DEFAULT false, deleted_at timestamptz) ON COMMIT DROP;
CREATE TEMP TABLE providers (id bigint, status text DEFAULT 'active', deleted_at timestamptz) ON COMMIT DROP;
CREATE TEMP TABLE provider_groups (provider_id bigint, group_id bigint) ON COMMIT DROP;
CREATE TEMP TABLE usage_logs (id bigint, group_id bigint, api_key_id bigint, request_id text, created_at timestamptz, input_tokens int DEFAULT 10, cache_creation_tokens int DEFAULT 10, cache_read_tokens int DEFAULT 80, stream bool DEFAULT true, first_token_ms int) ON COMMIT DROP;
CREATE TEMP TABLE ops_error_logs (id bigint, group_id bigint, api_key_id bigint, request_id text, created_at timestamptz, is_count_tokens bool DEFAULT false, status_code int DEFAULT 500, upstream_status_code int, upstream_errors jsonb, error_owner text DEFAULT 'provider', is_business_limited bool DEFAULT false) ON COMMIT DROP;
INSERT INTO groups(id,name,display_brand,rate_multiplier) VALUES (1,'Public','OpenAI',0.35),(2,'Private','OpenAI',1),(3,'Inactive','OpenAI',1),(4,'Deleted','OpenAI',1),(5,'No provider','OpenAI',1),(6,'No traffic','Anthropic',1),(7,'Inactive provider','OpenAI',1);
UPDATE groups SET is_exclusive=true WHERE id=2;
UPDATE groups SET status='disabled' WHERE id=3;
UPDATE groups SET deleted_at=now() WHERE id=4;
INSERT INTO providers(id,status) VALUES (1,'active'),(2,'disabled');
INSERT INTO provider_groups VALUES (1,1),(1,2),(1,3),(1,4),(1,6),(2,7);
INSERT INTO usage_logs(id,group_id,api_key_id,request_id,created_at,first_token_ms) VALUES
(1,1,1,'success-1','2038-01-01 11:59:00Z',1000),(2,1,1,'success-2','2038-01-01 11:59:01Z',3000),
(3,1,1,'success-1','2038-01-01 11:59:02Z',1000),(4,2,1,'private','2038-01-01 11:59:00Z',1),
(5,1,1,'outside','2038-01-01 12:00:00Z',999999),(6,1,1,'before','2038-01-01 10:29:59Z',999999),
(7,1,1,'start','2038-01-01 10:30:00Z',9000);
UPDATE usage_logs SET input_tokens=100,cache_creation_tokens=100,cache_read_tokens=0 WHERE id=2;
INSERT INTO ops_error_logs(id,group_id,api_key_id,request_id,created_at) VALUES
(1,1,1,'failed','2038-01-01 11:59:00Z'),(2,1,1,'failed','2038-01-01 11:59:01Z'),
(3,1,1,'success-1','2038-01-01 11:59:00Z'),(4,1,1,'business','2038-01-01 11:59:00Z'),
(5,1,1,'count','2038-01-01 11:59:00Z'),(6,1,1,'auth','2038-01-01 11:59:00Z'),
(7,1,1,'outside','2038-01-01 12:00:00Z');
UPDATE ops_error_logs SET is_business_limited=true WHERE id=4;
UPDATE ops_error_logs SET is_count_tokens=true WHERE id=5;
UPDATE ops_error_logs SET status_code=401,error_owner='client' WHERE id=6;
`)
	require.NoError(t, err)
	end := time.Date(2038, 1, 1, 12, 0, 0, 0, time.UTC)
	sla := opsSLACountableSQL("e.status_code", "e.upstream_status_code", "e.upstream_errors", "e.error_owner", "e.is_business_limited", nil)
	rows, err := transaction.QueryContext(ctx, fmt.Sprintf(channelMonitorSQL, sla), end.Add(-90*time.Minute), end, 270)
	require.NoError(t, err)
	defer rows.Close()
	seen := map[int64]int{}
	for rows.Next() {
		var id, requests, successes int64
		var name, brand string
		var rate float64
		var bucket int
		var cache, ttft sql.NullFloat64
		require.NoError(t, rows.Scan(&id, &name, &brand, &rate, &bucket, &requests, &successes, &cache, &ttft))
		seen[id]++
		if id == 1 {
			require.Contains(t, []int{-1, 0, 19}, bucket)
			switch bucket {
			case -1:
				require.Equal(t, int64(4), requests)
				require.Equal(t, int64(3), successes)
				require.Equal(t, float64(40), cache.Float64)
				require.Equal(t, float64(3000), ttft.Float64)
			case 0:
				require.Equal(t, int64(1), requests)
				require.Equal(t, int64(1), successes)
				require.Equal(t, float64(80), cache.Float64)
				require.Equal(t, float64(9000), ttft.Float64)
			case 19:
				require.Equal(t, int64(3), requests)
				require.Equal(t, int64(2), successes)
				require.InDelta(t, float64(80)/300*100, cache.Float64, 0.0001)
				require.Equal(t, float64(2000), ttft.Float64)
			}
		} else {
			require.Equal(t, int64(6), id)
			require.Zero(t, requests)
			require.False(t, cache.Valid)
		}
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[int64]int{1: 3, 6: 1}, seen)
}
