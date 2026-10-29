/**
 * pelican.ts supplies English interface messages for scheduled model drawing tests and
 * published drawing results. These locale keys are consumed by the corresponding pages
 * and controls, with interpolation values provided by the component displaying each
 * message.
 */

export default {
  title: 'Pelican Showcase',
  adminTitle: 'Pelican Tests',
  prompt: 'View current generation prompt',
  description:
    'Compare HTML / SVG animations generated from the same prompt. These examples are not standardized intelligence scores. History is retained for 30 days.',
  adminDescription:
    'Schedule generation by group and model. The platform pays upstream costs. Results passing format checks are published automatically.',
  group: 'Group',
  model: 'Model',
  date: 'Generation date (UTC+8)',
  all: 'All',
  refresh: 'Refresh',
  preview: 'View animation',
  loading: 'Loading…',
  loadFailed: 'Could not load. Refresh to retry.',
  empty: 'No artwork or runs yet.',
  previous: 'Previous',
  next: 'Next',
  viewGallery: 'Public gallery',
  create: 'Add test',
  edit: 'Edit test',
  save: 'Save',
  run: 'Run now',
  history: 'Run history',
  delete: 'Delete',
  details: 'Details',
  hide: 'Hide',
  publish: 'Publish',
  published: 'Published',
  hidden: 'Unpublished',
  paused: 'Paused',
  enabled: 'Enable scheduled tests',
  interval: 'Interval (minutes, 5 to 10080)',
  schedule: 'Schedule',
  lastRun: 'Last run',
  nextRun: 'Next run',
  actions: 'Actions',
  every: 'Every {minutes} minutes',
  queued: 'Queued',
  notRun: 'Not run yet',
  noTasks: 'No tests configured. New tests start paused.',
  newPaused: 'Save the new test in a paused state, then edit to enable scheduling or run it once.',
  consent:
    'I authorize public display of this group name, model name, future artwork, and success or failure records.',
  consentRequired: 'Confirm and save publication consent first.',
  cost: 'One generation per run, without automatic retries. Defaults: every 6 hours, 300-second timeout. Regular user balances are not charged.',
  deleteConfirm: 'Deletion cannot be undone. Deleting a test also deletes all its artwork.',
  provider: 'Execution provider ID',
  raw: 'Raw response',
  sandbox: 'Artwork runs in an isolated sandbox with external resources restricted.',
  status: {
    running: 'Running',
    success: 'Succeeded',
    failed: 'Failed',
    interrupted: 'Interrupted',
  },
}
