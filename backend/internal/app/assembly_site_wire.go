//go:build wireinject

package app

import (
	"github.com/google/wire"

	"github.com/TokenFlux/TokenRouter/internal/site"
	sitehttp "github.com/TokenFlux/TokenRouter/internal/site/httpapi"
	sitepostgres "github.com/TokenFlux/TokenRouter/internal/site/postgres"
)

// siteAssemblyProviders 汇总 site 模块的 Wire provider。
var siteAssemblyProviders = wire.NewSet(
	providePelicanService,
	sitepostgres.NewPelicanRepository,
	sitehttp.NewPelicanHandler,
	provideSiteDisplay,
	provideSitePublicHTTP,
	provideSitePublic,
	provideSitePages,
	site.NewAnnouncementService,
	site.NewMessageService,
	sitepostgres.NewMessageRepository,
	sitepostgres.NewAnnouncementRepository,
	sitepostgres.NewAnnouncementReadRepository,
	provideAnnouncementUsers,
	provideAnnouncementSubscriptions,
	provideAnnouncementExpiry,
)
