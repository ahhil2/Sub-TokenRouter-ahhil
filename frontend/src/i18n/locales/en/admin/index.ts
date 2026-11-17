import common from './common'
import providers from './providers'
import settings from './settings'
import ops from './ops'

// index.ts assembles the complete administrator message catalog from shared labels
// and the provider, settings, and monitoring domains. Complete imports supply every
// namespace for explicit catalog consumers, while route loading can request the
// common entry and the domains used by its page separately.

export default {
  ...common,
  ...providers,
  ...settings,
  ...ops,
}
