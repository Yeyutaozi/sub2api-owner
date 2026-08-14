import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import tokenRewards from './tokenRewards'
import promptAudit from './promptAudit'
import videoJobs from './videoJobs'
import imageJobs from './imageJobs'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...tokenRewards,
  ...promptAudit,
  ...videoJobs,
  ...imageJobs,
}
