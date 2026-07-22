import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Seedance account type', () => {
  it('creates a dedicated API-key account with the FYLink default', () => {
    expect(source).toContain('@click="form.platform = \'seedance\'"')
    expect(source).toContain("if (form.platform === 'seedance')")
    expect(source).toContain("accountCategory.value = 'apikey'")
    expect(source).toContain("form.type = 'apikey'")
    expect(source).toContain("? 'https://api.fflink.top'")
    expect(source).toContain("? 'Sub2API Key'")
  })

  it('does not expose Seedance as an OpenAI endpoint capability', () => {
    expect(source).not.toContain('seedance_proxy')
  })
})
