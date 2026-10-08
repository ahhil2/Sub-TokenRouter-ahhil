//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/usage"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// TestResponseModelMigrationPreservesHistory 在事务内还原旧列结构，确认增量迁移保留历史空值。
func (s *UsageLogRepoSuite) TestResponseModelMigrationPreservesHistory() {
	user := mustCreateUser(s.T(), s.client, &identity.User{Email: "response-history@test.local"})
	key := mustCreateApiKey(s.T(), s.client, &apikey.APIKey{UserID: user.ID, Key: "response-history", Name: "history"})
	upstream := mustCreateProvider(s.T(), s.client, &providercore.Record{Name: "response-history"})
	log := s.createUsageLog(user, key, upstream, 1, 1, 0, time.Now())
	_, err := s.tx.ExecContext(s.ctx, "ALTER TABLE usage_logs DROP COLUMN upstream_response_model, DROP COLUMN upstream_model_mismatch")
	s.Require().NoError(err)
	migration, err := migrations.FS.ReadFile("285_usage_upstream_response_model.sql")
	s.Require().NoError(err)
	for range 2 {
		_, err = s.tx.ExecContext(s.ctx, string(migration))
		s.Require().NoError(err)
	}
	loaded, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err)
	s.Nil(loaded.UpstreamResponseModel)
	s.Nil(loaded.UpstreamModelMismatch)
}

func TestUsageLog_ListWithFilters_ResolvesSoftDeletedUser(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := NewUsageLogRepositoryWithSQL(client, tx, timezone.NewCalendar(time.Local))

	// 一个活跃用户、一个将被软删的用户，各一条日志。
	active := mustCreateUser(t, client, &identity.User{Email: "active-listfilter@test.com"})
	deleted := mustCreateUser(t, client, &identity.User{Email: "deleted-listfilter@test.com"})
	apiKey := mustCreateApiKey(t, client, &apikey.APIKey{UserID: deleted.ID, Key: "sk-del-1", Name: "k"})
	apiKey2 := mustCreateApiKey(t, client, &apikey.APIKey{UserID: active.ID, Key: "sk-act-1", Name: "k"})
	provider := mustCreateProvider(t, client, &providercore.Record{Name: "acc-listfilter"})

	now := time.Now().UTC()
	for _, u := range []struct {
		uid int64
		kid int64
	}{{deleted.ID, apiKey.ID}, {active.ID, apiKey2.ID}} {
		_, err := repo.Create(ctx, &usage.UsageLog{
			UserID: u.uid, APIKeyID: u.kid, ProviderID: provider.ID,
			Model: "claude-3", InputTokens: 1, OutputTokens: 1,
			TotalCost: 0.1, ActualCost: 0.1, CreatedAt: now,
		})
		require.NoError(t, err)
	}

	// 软删除该用户（触发 SoftDeleteMixin Hook → UPDATE deleted_at）。
	require.NoError(t, client.User.DeleteOneID(deleted.ID).Exec(ctx))

	logs, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50},
		usage.UsageLogFilters{ExactTotal: true})
	require.NoError(t, err)

	byUser := map[int64]usage.UsageLog{}
	for _, l := range logs {
		byUser[l.UserID] = l
	}

	// 已删用户的日志加载用户资料后，包含邮箱和删除时间。
	delLog, ok := byUser[deleted.ID]
	require.True(t, ok, "deleted user's usage log must still be listed")
	require.NotNil(t, delLog.User, "deleted user identity must resolve")
	require.Equal(t, "deleted-listfilter@test.com", delLog.User.Email)
	require.NotNil(t, delLog.User.DeletedAt, "DeletedAt must be set for soft-deleted user")

	// 活跃用户：DeletedAt 为 nil。
	actLog := byUser[active.ID]
	require.NotNil(t, actLog.User)
	require.Nil(t, actLog.User.DeletedAt)
}

func (s *UsageLogRepoSuite) TestListWithFilters_SortByModelAsc() {
	user := mustCreateUser(s.T(), s.client, &identity.User{Email: "usage-sort@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &apikey.APIKey{UserID: user.ID, Key: "sk-usage-sort", Name: "k"})
	provider := mustCreateProvider(s.T(), s.client, &providercore.Record{Name: "usage-sort-provider"})

	first := &usage.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		ProviderID:     provider.ID,
		RequestID:      uuid.New().String(),
		Model:          "z-model",
		RequestedModel: "z-model",
		InputTokens:    10,
		OutputTokens:   20,
		TotalCost:      0.5,
		ActualCost:     0.5,
		CreatedAt:      time.Now(),
	}
	_, err := s.repo.Create(s.ctx, first)
	s.Require().NoError(err)

	second := &usage.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		ProviderID:     provider.ID,
		RequestID:      uuid.New().String(),
		Model:          "a-model",
		RequestedModel: "a-model",
		InputTokens:    10,
		OutputTokens:   20,
		TotalCost:      0.5,
		ActualCost:     0.5,
		CreatedAt:      time.Now().Add(time.Second),
	}
	_, err = s.repo.Create(s.ctx, second)
	s.Require().NoError(err)

	logs, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "model",
		SortOrder: "asc",
	}, usage.UsageLogFilters{UserID: user.ID})
	s.Require().NoError(err)
	s.Require().Len(logs, 2)
	s.Require().Equal("a-model", logs[0].RequestedModel)
	s.Require().Equal("z-model", logs[1].RequestedModel)
}
