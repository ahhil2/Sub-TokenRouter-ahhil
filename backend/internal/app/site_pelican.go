package app

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/selection"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

type pelicanExecutor struct {
	groups *routing.GroupAdmin
	probe  selection.Probe
	tests  *provider.TestService
}

func providePelicanService(repository site.PelicanRepository, groups *routing.GroupAdmin, tests *provider.TestService,
	gateway *selection.Generic, openai *selection.Compatible, gemini *selection.Gemini, manager *lifecycle.Manager,
) *site.PelicanService {
	executor := pelicanExecutor{groups: groups, probe: selection.NewProbe(tests, gateway, openai, gemini), tests: tests}
	service := site.NewPelicanService(repository, executor)
	runner := site.NewPelicanRunner(service)
	manager.Register(lifecycle.Hook{Name: "PelicanRunner", StartOrder: 980, StopOrder: 20, Start: runner.Start, Stop: runner.Stop})
	return service
}

func (executor pelicanExecutor) Validate(ctx context.Context, groupID int64, model string) error {
	group, err := executor.groups.GetGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if group.Status != "active" {
		return site.ErrPelicanInvalid
	}
	models, err := executor.groups.GetGroupModelsListCandidates(ctx, groupID, "")
	if err != nil {
		return err
	}
	if !routing.ModelsListCandidateAllowsModel(models, model) {
		return site.ErrPelicanInvalid
	}
	return nil
}

func (executor pelicanExecutor) Generate(ctx context.Context, task site.PelicanTask) (site.PelicanExecution, error) {
	target, err := executor.probe.Select(ctx, routing.GroupAvailabilityProbeDueGroup{GroupID: task.GroupID}, task.Model)
	if err != nil {
		return site.PelicanExecution{}, err
	}
	prompt, err := site.NewPelicanGenerationPrompt()
	if err != nil {
		return site.PelicanExecution{}, err
	}
	generated, err := executor.tests.GenerateText(ctx, target.ProviderID, target.ModelID, prompt, site.PelicanOutputTokens, site.PelicanMaxResponseBytes)
	return site.PelicanExecution{Text: generated.Text, ProviderID: target.ProviderID, Model: generated.Model}, err
}
