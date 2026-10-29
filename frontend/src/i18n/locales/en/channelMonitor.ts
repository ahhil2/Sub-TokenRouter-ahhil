/**
 * channelMonitor.ts supplies English interface messages for public channel metrics and
 * administrator monitoring settings. These locale keys are consumed by the
 * corresponding pages and controls, with interpolation values provided by the
 * component displaying each message.
 */

export default {
  title: 'Channel monitoring',
  subtitle: 'Request statistics for the last 90 minutes, refreshed every 30 seconds',
  realRequests: 'Live requests',
  multiplier: 'Rate',
  multiplierHint:
    'Default public group rate. Personal discounts and final charges follow actual billing.',
  cacheRate: 'Cache rate',
  availability: 'Availability',
  firstToken: 'First token',
  history: 'Live requests · {count} periods · {minutes} min each',
  historyLabel: 'Channel availability over the last 90 minutes',
  refreshIn: 'Refresh in {seconds}s',
  refresh: 'Refresh',
  refreshing: 'Refreshing',
  updatedAt: 'Updated at {time}',
  requests: '{count} requests',
  sampleCount: '{successes} / {requests} requests succeeded',
  noRequests: 'No requests in this period',
  loading: 'Loading channel monitoring…',
  empty: 'No public channels to display',
  disabled: 'Channel monitoring is disabled',
  loadFailed: 'Could not load monitoring data. Please retry.',
  stale: 'Refresh failed. Showing the previous snapshot; check its update time.',
  configure: 'Monitor settings',
  configHint:
    'Select groups to display. New groups are hidden by default. Status can follow request statistics or be set manually. Manual status is labeled; metrics and history blocks still reflect actual requests. Up to 200 groups can be displayed.',
  configLoadFailed: 'Could not load settings. Please retry.',
  configSaveFailed:
    'Could not save settings. Check that selected groups still exist and that no more than 200 are selected, then retry.',
  ineligible:
    'Hidden from users: the group must be active, non-exclusive, and linked to an active provider. Its settings are retained.',
  groupStatus: 'Display status for {name}',
  noGroups: 'No groups to configure',
  save: 'Save settings',
  saving: 'Saving',
  saved: 'Settings saved',
  manual: 'Manual',
  manualStatusHint:
    'Status set by an administrator. Metrics and history blocks reflect actual requests.',
  status: {
    auto: 'Automatic (request statistics)',
    maintenance: 'Maintenance',
    operational: 'Normal',
    degraded: 'Degraded',
    failed: 'Failed',
    unknown: 'No data',
  },
  availabilityDefinition:
    'Availability = successful requests / (successful + failed requests). Excludes business limits and 401/403 without upstream errors. Normal: ≥95%; failed: 0%; otherwise degraded.',
  cacheDefinition:
    'Cache rate = cache-read tokens / (input + cache-write + cache-read tokens). No samples are shown as —.',
  ttftDefinition:
    'First-token P50 is the median time to first token for successful streaming requests. Periods use recorded request timestamps and your browser timezone.',
}
