package ops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// release_query_test.go verifies update discovery, cache reuse, and eligible
// rollback selection with deterministic metadata fixtures. Asset checks retain
// authenticated API download endpoints through cache serialization while older
// metadata continues to provide its browser URL.

// updateServiceCacheStub supplies one mutable serialized release value for
// query fixtures.
type updateServiceCacheStub struct {
	// data is the latest value written by SetUpdateInfo, with empty text
	// interpreted as a cache miss. The fixture ignores expiration and is used
	// sequentially.
	data string
}

// updateServiceGitHubClientStub returns supplied metadata records for
// deterministic release-query scenarios.
type updateServiceGitHubClientStub struct {
	// release is returned unchanged by FetchLatestRelease with nil error,
	// including nil when unset. Tests that invoke latest lookup supply a usable
	// record for its dereferencing consumer.
	release *GitHubRelease
	// recentReleases is the supplied slice returned by recent-page lookup without
	// copying records or their assets. Candidate queries retain some of those same
	// pointers.
	recentReleases []*GitHubRelease
	// recentErr is returned alongside recentReleases, even when that slice is
	// nonempty. The query discards records when this configured error is nonnil.
	recentErr error
}

// GetUpdateInfo returns fixture data or a cache-miss error when it is empty.
// Context and cache expiration are not modeled here, leaving JSON age checking
// to the query under test.
func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

// SetUpdateInfo replaces the fixture data and returns nil while ignoring
// context and ttl. Subsequent reads see the supplied text until another fixture
// write changes it; no expiration worker runs.
func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

// FetchLatestRelease returns the supplied release pointer and nil error,
// ignoring repository and context. It performs no network read or copying;
// unset fixtures can return nil and are unsuitable for latest-result
// dereferencing.
func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

// TestReleaseQueryRetainsAssetAPIURLs discovers a release containing both current
// and legacy asset metadata. Fresh and cached results must select authenticated
// API URLs where available and keep older browser-only assets downloadable.
func TestReleaseQueryRetainsAssetAPIURLs(t *testing.T) {
	query := NewReleaseQuery(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{
		release: &GitHubRelease{TagName: "v0.4.8", Assets: []GitHubAsset{
			{
				Name:               "checksums.txt",
				APIURL:             "https://api.github.com/repos/test/private/releases/assets/123",
				BrowserDownloadURL: "https://github.com/test/private/releases/download/v0.4.8/checksums.txt",
			},
			{
				Name:               "legacy.tar.gz",
				BrowserDownloadURL: "https://github.com/test/private/releases/download/v0.4.8/legacy.tar.gz",
			},
		}},
	}, "0.4.7", "release")
	for _, force := range []bool{true, false} {
		info, err := query.CheckUpdate(context.Background(), force)
		require.NoError(t, err)
		require.True(t, info.HasUpdate)
		require.Equal(t, !force, info.Cached)
		require.Equal(t, "https://api.github.com/repos/test/private/releases/assets/123", info.ReleaseInfo.Assets[0].DownloadURL)
		require.Equal(t, "https://github.com/test/private/releases/download/v0.4.8/legacy.tar.gz", info.ReleaseInfo.Assets[1].DownloadURL)
	}
}

// TestReleaseQueryFourComponentVersions checks revision updates through fresh
// metadata and cached reads. Numeric ordering must include the fourth component
// while treating an omitted revision as zero and rejecting older releases.
func TestReleaseQueryFourComponentVersions(t *testing.T) {
	cases := []struct {
		name      string // version comparison scenario
		current   string // installed version text
		latest    string // published version text
		hasUpdate bool   // expected update eligibility
	}{
		{name: "revision after three components", current: "0.4.17", latest: "0.4.17.1", hasUpdate: true},
		{name: "numeric revision ordering", current: "0.4.17.2", latest: "0.4.17.10", hasUpdate: true},
		{name: "patch outranks revision", current: "0.4.16.9", latest: "0.4.17", hasUpdate: true},
		{name: "zero revision equals three components", current: "0.4.17", latest: "0.4.17.0"},
		{name: "three components equal zero revision", current: "0.4.17.0", latest: "0.4.17"},
		{name: "older revision", current: "0.4.17.10", latest: "0.4.17.2"},
		{name: "older three component release", current: "0.4.17.1", latest: "0.4.17"},
		{name: "older patch with revision", current: "0.4.17", latest: "0.4.16.99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v" + tc.latest}}
			query := NewReleaseQuery(&updateServiceCacheStub{}, client, tc.current, "release")
			for _, force := range []bool{true, false} {
				info, err := query.CheckUpdate(context.Background(), force)
				require.NoError(t, err)
				require.Equal(t, tc.current, info.CurrentVersion)
				require.Equal(t, tc.latest, info.LatestVersion)
				require.Equal(t, tc.hasUpdate, info.HasUpdate)
				require.Equal(t, !force, info.Cached)
				require.Empty(t, info.Warning)
				// Change the remote fixture after population so the second read must
				// use cached metadata rather than accidentally repeat the release fetch.
				client.release = &GitHubRelease{TagName: "v99.0.0"}
			}
		})
	}
}

// TestReleaseQueryCachedRevisionRechecksRunningVersion shares cached release
// metadata across instances with different installed revisions. Each instance
// must recompute update eligibility from its own running version.
func TestReleaseQueryCachedRevisionRechecksRunningVersion(t *testing.T) {
	cache := &updateServiceCacheStub{}
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.4.17.2"}}
	query := NewReleaseQuery(cache, client, "0.4.17", "release")
	info, err := query.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	client.release = &GitHubRelease{TagName: "v99.0.0"}
	for _, tc := range []struct {
		current   string // installed version text
		hasUpdate bool   // expected cached eligibility
	}{
		{current: "0.4.17.1", hasUpdate: true},
		{current: "0.4.17.2"},
		{current: "0.4.17.3"},
	} {
		t.Run(tc.current, func(t *testing.T) {
			otherInstance := NewReleaseQuery(cache, client, tc.current, "release")
			cached, err := otherInstance.CheckUpdate(context.Background(), false)
			require.NoError(t, err)
			require.True(t, cached.Cached)
			require.Equal(t, tc.current, cached.CurrentVersion)
			require.Equal(t, "0.4.17.2", cached.LatestVersion)
			require.Equal(t, tc.hasUpdate, cached.HasUpdate)
		})
	}
}

// FetchRecentReleases returns the supplied recent slice and configured error
// while ignoring repository, page count, and context. Records remain shared, so
// tests keep their fixture metadata stable during candidate selection.
func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

// DownloadFile panics unconditionally to reject asset transfers through this
// release-query fixture. The method implements no download behavior; metadata
// query services never call it, and installation tests need a downloader
// fixture.
func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

// FetchChecksumFile panics unconditionally when asked for checksum bytes
// through this metadata fixture. Query construction supplies only metadata
// methods, and callers use a checksum-capable fixture when exercising
// installation rather than release selection.
func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

// newRollbackTestService retains the supplied current version and recent
// release pointers in a ReleaseQuery with an empty cache and release build
// label. It performs no request; tests keep those borrowed records stable while
// querying.
func newRollbackTestService(current string, releases []*GitHubRelease) *ReleaseQuery {
	return NewReleaseQuery(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

// TestUpdateServiceListRollbackVersionsFiltersAndCaps supplies current/newer
// versions, a prerelease, a draft, duplicates, and more eligible history than
// the display cap. It checks the newest three older stable normalized versions
// remain.
func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // Exclude versions newer than the current version.
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // Exclude the current version.
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // Exclude prereleases.
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // Exclude drafts.
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // Exclude duplicate versions.
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // Exclude entries beyond the three-version limit.
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

// TestUpdateServiceListRollbackVersionsSortsUnorderedInput supplies three
// eligible older releases in mixed numeric order. It checks display values are
// ordered newest first independently of the fixture response sequence.
func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

// TestReleaseQueryRollbackRevisionsSortAndDeduplicate supplies mixed release
// spellings and unordered revisions. Selection must compare revisions numerically
// and retain the first release record for equivalent three- and four-part versions.
func TestReleaseQueryRollbackRevisionsSortAndDeduplicate(t *testing.T) {
	cases := []struct {
		name     string   // rollback selection scenario
		current  string   // installed version text
		tags     []string // returned release tag order
		expected []string // expected candidate tag order
	}{
		{
			name: "revision ordering and cap", current: "0.4.17.11",
			tags:     []string{"v0.4.17.2", "v0.4.16.99", "v0.4.17.10", "v0.4.17.1", "v0.4.17.11", "v0.4.18"},
			expected: []string{"v0.4.17.10", "v0.4.17.2", "v0.4.17.1"},
		},
		{
			name: "zero revision appears first", current: "0.4.17.2",
			tags:     []string{"v0.4.17.0", "v0.4.17", "v0.4.16.9", "v0.4.17.1"},
			expected: []string{"v0.4.17.1", "v0.4.17.0", "v0.4.16.9"},
		},
		{
			name: "three components appear first", current: "0.4.17.2",
			tags:     []string{"v0.4.17", "v0.4.17.0", "v0.4.16.9", "v0.4.17.1"},
			expected: []string{"v0.4.17.1", "v0.4.17", "v0.4.16.9"},
		},
		{
			name: "current zero revision is excluded", current: "0.4.17",
			tags:     []string{"v0.4.17.0", "v0.4.17.1", "v0.4.16.10", "v0.4.16.2"},
			expected: []string{"v0.4.16.10", "v0.4.16.2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			releases := make([]*GitHubRelease, 0, len(tc.tags))
			for _, tag := range tc.tags {
				releases = append(releases, &GitHubRelease{TagName: tag, Name: "Release " + tag})
			}
			candidates, err := newRollbackTestService(tc.current, releases).FetchRollbackCandidates(context.Background())
			require.NoError(t, err)
			actual := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				actual = append(actual, candidate.TagName)
				require.Equal(t, "Release "+candidate.TagName, candidate.Name)
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestNormalizeRollbackVersionFourthComponent checks supported revision text
// and malformed fourth components. Canonical three- and four-part values remain
// usable, while extra components and unsafe spellings are rejected.
func TestNormalizeRollbackVersionFourthComponent(t *testing.T) {
	for _, tc := range []struct {
		input    string // submitted rollback version
		expected string // accepted normalized spelling
	}{
		{input: "0.4.17", expected: "0.4.17"},
		{input: "0.4.17.0", expected: "0.4.17.0"},
		{input: " v0.4.17.10 ", expected: "0.4.17.10"},
		{input: "0.4.17."},
		{input: "0.4.17.one"},
		{input: "0.4.17.-1"},
		{input: "0.4.17.01"},
		{input: "0.4.17.1+build"},
		{input: "0.4.17.1-rc1"},
		{input: "0.4.17.1.2"},
		{input: "0.4.17.999999999999999999999999999999999999"},
		{input: "0.4.17.1;echo unsafe"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			version, ok := NormalizeRollbackVersion(tc.input)
			require.Equal(t, tc.expected != "", ok)
			require.Equal(t, tc.expected, version)
		})
	}
}

// TestUpdateServiceListRollbackVersionsRejectsNonSemverTags supplies a numeric
// release alongside shell-like text, an alternate prefix, leading zeros, and
// build metadata. It checks strict rollback spelling accepts only the listed
// canonical numeric version.
func TestUpdateServiceListRollbackVersionsRejectsNonSemverTags(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145;$(touch /tmp/tokenrouter-pwned)"},
		{TagName: "release-0.1.144"},
		{TagName: "v0.1.0143"},
		{TagName: "v0.1.142+meta"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Equal(t, []RollbackVersion{{Version: "0.1.146"}}, versions)
}

// TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder supplies the running
// version and a newer release as its entire recent page. It checks candidate
// display returns an empty result with no error.
func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

// TestUpdateServiceListRollbackVersionsPropagatesFetchError injects a
// recent-release reader error for a valid running version. It checks the
// display query returns that failure text instead of a successful candidate
// list.
func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewReleaseQuery(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}
