import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
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
})
