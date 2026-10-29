//go:build wireinject

package app

import (
	"github.com/google/wire"

	"github.com/TokenFlux/TokenRouter/internal/site"
	sitehttp "github.com/TokenFlux/TokenRouter/internal/site/httpapi"
	sitepostgres "github.com/TokenFlux/TokenRouter/internal/site/postgres"
)

// assembly_site_wire.go registers public site, page, announcement, audience,
// and expiry dependencies with Wire.
// Announcement audience checks read users and active billing subscriptions
// through application adapters. Public site information reuses shared settings
// and OAuth settings, while announcement persistence and expiry constructors
// join the same application graph used by site HTTP routes.

// siteAssemblyProviders supplies public site and page services alongside
// announcement persistence, audience readers, and expiry handling.
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
