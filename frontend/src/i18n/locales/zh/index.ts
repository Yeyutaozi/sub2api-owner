import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import creazyCanvas from './creazyCanvas'
import admin from './admin'
import misc from './misc'
import tokenRewards from './tokenRewards'
import compat from './compat'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...creazyCanvas,
  ...tokenRewards,
  ...misc,
  keys: { ...dashboard.keys, ...compat.keys, useKeyModal: { ...dashboard.keys.useKeyModal, ...compat.keys.useKeyModal } },
  modelPlaza: { ...dashboard.modelPlaza, ...compat.modelPlaza, table: { ...dashboard.modelPlaza.table, ...compat.modelPlaza.table } },
  monitorCommon: { ...dashboard.monitorCommon, ...compat.monitorCommon, providers: { ...dashboard.monitorCommon.providers, ...compat.monitorCommon.providers } },
  redeem: { ...dashboard.redeem, ...compat.redeem },
  usage: { ...dashboard.usage, ...compat.usage },
  admin: {
    ...admin,
    accounts: { ...admin.accounts, ...compat.admin.accounts, messages: { ...admin.accounts.messages, ...compat.admin.accounts.messages } },
    groups: { ...admin.groups, ...compat.admin.groups, creazyCanvas: { ...admin.groups.creazyCanvas, ...compat.admin.groups.creazyCanvas }, modelsList: { ...admin.groups.modelsList, ...compat.admin.groups.modelsList } },
  },
}
