import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { createSSRApp, defineComponent, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { compileScript, compileTemplate, parse } from 'vue/compiler-sfc'

const sourcePath = join(dirname(fileURLToPath(import.meta.url)), 'index.vue')

async function renderModal(headerExtra: string) {
  const source = readFileSync(sourcePath, 'utf8')
  const parsed = parse(source)
  const script = compileScript(parsed.descriptor, { id: 'round-card-modal' })
  const compiled = compileTemplate({
    filename: sourcePath,
    id: 'round-card-modal',
    source: parsed.descriptor.template?.content ?? '',
    compilerOptions: { bindingMetadata: script.bindings },
  })
  assert.deepEqual(compiled.errors, [])
  const body = compiled.code
    .replace(/import \{[\s\S]+?\} from "vue"\n/, '')
    .replace('export function render', 'function render')
  const vue = await import('vue')
  const render = new Function('vue', `
    const _renderSlot = vue.renderSlot
    const _mergeProps = vue.mergeProps
    const _withCtx = vue.withCtx
    const _openBlock = vue.openBlock
    const _createBlock = vue.createBlock
    ${body}
    return render
  `)(vue)

  const received: Record<string, string> = {}
  const textOf = (value: unknown): string => {
    if (typeof value === 'string' || typeof value === 'number')
      return String(value)
    if (Array.isArray(value))
      return value.map(textOf).join('')
    if (value && typeof value === 'object' && 'children' in value)
      return textOf((value as { children?: unknown }).children)
    return ''
  }
  const StubModal = defineComponent({
    name: 'NModal',
    setup(_, { slots }) {
      return () => {
        for (const name of ['header', 'header-extra', 'eader-extra', 'default'] as const)
          received[name] = textOf(slots[name]?.())
        return h('section')
      }
    },
  })

  const Modal = defineComponent({
    props: { show: Boolean, title: String, size: String },
    setup() {
      return { NModal: StubModal, showModal: true, bindAttrs: { class: '', style: '' } }
    },
    render,
  })

  const app = createSSRApp({
    components: { Modal },
    setup() {
      return () => h(Modal, { show: true, title: 'Title' }, {
        header: () => 'HEAD',
        'header-extra': () => headerExtra,
        default: () => 'BODY',
      })
    },
  })
  await renderToString(app)
  return received
}

test('header-extra content is forwarded to the modal header-extra slot', async () => {
  const slots = await renderModal('EXTRA')
  assert.equal(slots['header-extra'], 'EXTRA')
  assert.equal(slots['eader-extra'], '')
  assert.equal(slots.header, 'HEAD')
  assert.equal(slots.default, 'BODY')
})
