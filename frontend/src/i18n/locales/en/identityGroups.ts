/**
 * identityGroups.ts supplies English interface messages for identity membership, rate
 * overrides, balance discounts, and historical recharge gifts. These locale keys are consumed by the
 * corresponding pages and controls, with interpolation values provided by the
 * component displaying each message.
 */

export default {
  title: 'Identity groups',
  ordinary: 'Routing groups',
  members: 'Manage members',
  rates: 'Set rates',
  count: '{count} members',
  discount:
    'Balance usage is billed at {percent}% of the routing group default rate. Personal and AHHilAI rates take precedence.',
  pricing: 'Set rates for routing groups. Personal rates take precedence.',
  search: 'Search username, email or user ID',
  searchButton: 'Search',
  addMode: 'Find users to add',
  memberMode: 'View members',
  add: 'Add identity',
  remove: 'Remove identity',
  selected: '{count} users selected',
  replace:
    'A user can hold only one of the 95% and 90% discount identities; assigning one replaces the other. AHHilAI can coexist.',
  previous: 'Previous',
  next: 'Next',
  page: 'Page {page}, {total} users',
  empty: 'No matching users',
  inherit: 'Inherit',
  defaultRate: 'Default rate',
  identityRate: 'Identity rate',
  group: 'Group',
  clear: 'Inherit all',
  rateHelp:
    'An empty value inherits the routing group rate. Zero means free. Personal rates take precedence.',
  save: 'Save',
  close: 'Close',
  loading: 'Loading…',
  saved: 'Saved',
  failed: 'Operation failed. Please retry.',
  invalid: 'Enter a finite, nonnegative rate.',
  assign: 'Assign identity',
  filter: 'All identities',
  benefits: 'My identity benefits',
  none: 'No special identity',
  base: 'Base credit',
  gift: 'Bonus credit',
  credited: 'Total credit',
  refund: 'Refunds also reclaim the proportional bonus.',
  select: 'Select user',
}
