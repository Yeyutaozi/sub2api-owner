import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)
const editSource = readFileSync(
  resolve(process.cwd(), 'src/components/account/EditAccountModal.vue'),
  'utf8'
)
const providerSource = readFileSync(
  resolve(process.cwd(), 'src/utils/videoAccountProviders.ts'),
  'utf8'
)

describe('CreateAccountModal video platform account types', () => {
  it('creates dedicated API-key accounts with the internal video upstream default', () => {
    expect(source).toContain('@click="form.platform = \'seedance\'"')
    expect(source).toContain('@click="form.platform = \'ltx\'"')
    expect(source).toContain('@click="form.platform = \'happyhorse\'"')
    expect(source).toContain("form.platform === 'seedance' || form.platform === 'ltx' || form.platform === 'happyhorse' || form.platform === 'glm'")
    expect(source).toContain("accountCategory.value = 'apikey'")
    expect(source).toContain("form.type = 'apikey'")
    expect(source).toContain("? 'https://api.fflink.top'")
    expect(source).toContain("? 'Sub2API Key'")
  })

  it('does not expose video platforms as OpenAI endpoint capabilities', () => {
    expect(source).not.toContain('seedance_proxy')
    expect(source).not.toContain('ltx_proxy')
    expect(source).not.toContain('happyhorse_proxy')
  })

  it('persists the internal Seedance video provider on create and edit', () => {
    expect(source).toContain('v-model="seedanceVideoProvider"')
    expect(source).toContain('<option value="fflink">')
    expect(source).toContain('<option value="huiqu">')
    expect(source).toContain("const seedanceVideoProvider = ref<SeedanceVideoProvider>('fflink')")
    expect(source).toContain('credentials.video_provider = seedanceVideoProvider.value')
    expect(source).toContain('apiKeyBaseUrl.value = getSeedanceVideoProviderBaseUrl(seedanceVideoProvider.value)')

    expect(editSource).toContain('data-testid="edit-seedance-video-provider"')
    expect(editSource).toContain("credentials?.video_provider === 'huiqu'")
    expect(editSource).toContain('newCredentials.video_provider = seedanceVideoProvider.value')
    expect(editSource).toContain('editBaseUrl.value = getSeedanceVideoProviderBaseUrl(seedanceVideoProvider.value)')
    expect(providerSource).toContain("huiqu: 'https://api.bjhuiqu.net'")
  })

  it('requires an explicit model mapping for every video account', () => {
    expect(source).toContain("t('admin.accounts.videoModelMappingRequired')")
    expect(source).toContain('isVideoAccountPlatform.value && !modelMapping')
    expect(editSource).toContain("t('admin.accounts.videoModelMappingRequired')")
    expect(editSource).toContain('isVideoAccountPlatform.value && !modelMapping')
  })
})
