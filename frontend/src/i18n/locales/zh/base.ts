import auth from './auth'
import dashboard from './dashboard'
import batchImage from './batchImage'
import creative from './creative'
import admin from './admin/common'
import misc from './misc'
import team from './team'
import requests from './requests'
import pelican from './pelican'
import channelMonitor from './channelMonitor'
import identityGroups from './identityGroups'

// base.ts assembles the Simplified Chinese interface catalog from domain
// dictionaries and shared legal and localization messages. Named request and
// administrator namespaces coexist with root-level spreads, whose order
// determines the value retained if domain modules define the same key.

export default {
  identityGroups,
  channelMonitor,
  pelican,
  requests,
  ...auth,
  ...dashboard,
  ...batchImage,
  ...creative,
  admin,
  ...misc,
  ...team,
}
