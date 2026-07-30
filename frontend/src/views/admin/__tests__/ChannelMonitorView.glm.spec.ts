import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'
import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import {
  API_MODE_CHAT_COMPLETIONS,
  DEFAULT_GLM_ENDPOINT,
  DEFAULT_GLM_MODEL,
  DEFAULT_GROK_ENDPOINT,
  DEFAULT_GROK_MODEL,
  PROVIDERS,
  PROVIDER_GLM,
} from '@/constants/channelMonitor'

const { createMonitor, listTemplates } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  listTemplates: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: vi.fn(),
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

function mountDialog() {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: null },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        Select: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
}

describe('channel monitor GLM provider', () => {
  beforeEach(() => {
    createMonitor.mockReset().mockResolvedValue(undefined)
    listTemplates.mockReset().mockResolvedValue({ items: [] })
  })

  it('offers a distinct GLM provider with official defaults and no Responses mode', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(PROVIDERS).toContain(PROVIDER_GLM)
    const providerButtons = wrapper.findAll('[data-testid^="monitor-provider-"]')
    expect(providerButtons).toHaveLength(PROVIDERS.length)

    const glmButton = wrapper.get('[data-testid="monitor-provider-glm"]')
    expect(glmButton.find('svg').exists()).toBe(true)
    expect(glmButton.text()).toContain('monitorCommon.providers.glm')

    await glmButton.trigger('click')

    expect(glmButton.classes().join(' ')).toContain('indigo')
    expect(wrapper.text()).not.toContain('admin.channelMonitor.form.apiModeResponses')
    expect((wrapper.get('[data-testid="monitor-endpoint"]').element as HTMLInputElement).value)
      .toBe(DEFAULT_GLM_ENDPOINT)
    expect((wrapper.get('[data-testid="monitor-primary-model"]').element as HTMLInputElement).value)
      .toBe(DEFAULT_GLM_MODEL)
  })

  it('keeps provider defaults isolated and preserves administrator-entered values', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')

    await wrapper.get('[data-testid="monitor-provider-glm"]').trigger('click')
    await wrapper.get('[data-testid="monitor-provider-grok"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await wrapper.get('[data-testid="monitor-provider-glm"]').trigger('click')
    await endpoint.setValue('https://glm-gateway.example.com/v4')
    await model.setValue('glm-custom')
    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://glm-gateway.example.com/v4')
    expect((model.element as HTMLInputElement).value).toBe('glm-custom')
  })

  it('submits GLM monitors with Chat Completions semantics', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-glm"]').trigger('click')
    await wrapper
      .get('input[placeholder="admin.channelMonitor.form.namePlaceholder"]')
      .setValue('GLM health')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      name: 'GLM health',
      provider: PROVIDER_GLM,
      api_mode: API_MODE_CHAT_COMPLETIONS,
      endpoint: DEFAULT_GLM_ENDPOINT,
      primary_model: DEFAULT_GLM_MODEL,
    }))
  })

  it('uses an OpenAI-compatible Chat Completions body example', () => {
    const wrapper = mount(MonitorAdvancedRequestConfig, {
      props: {
        provider: PROVIDER_GLM,
        apiMode: API_MODE_CHAT_COMPLETIONS,
        extraHeaders: {},
        bodyOverrideMode: 'replace',
        bodyOverride: null,
      },
    })

    const placeholder = wrapper.get('textarea').attributes('placeholder')
    expect(placeholder).toContain(`"model": "${DEFAULT_GLM_MODEL}"`)
    expect(placeholder).toContain('"messages"')
    expect(placeholder).not.toContain('"input"')
  })
})
