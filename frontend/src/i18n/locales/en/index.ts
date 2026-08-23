import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import creazyCanvas from './creazyCanvas'
import admin from './admin'
import misc from './misc'
import tokenRewards from './tokenRewards'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...creazyCanvas,
  admin,
  ...tokenRewards,
  ...misc,
}
