import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import creazyCanvas from './creazyCanvas'
import admin from './admin'
import misc from './misc'
import tokenRewards from './tokenRewards'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  ...creazyCanvas,
  admin,
  ...tokenRewards,
  ...misc,
}
