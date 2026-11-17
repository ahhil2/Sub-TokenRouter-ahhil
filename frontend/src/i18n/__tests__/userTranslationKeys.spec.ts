import { describe, expect, it, vi } from 'vitest'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import en from '../locales/en'
import zh from '../locales/zh'
import enBase from '../locales/en/base'
import zhBase from '../locales/zh/base'
import manifest from '../../../../backend/internal/pkg/locale/manifest.json'
import ts from 'typescript'
import { compileTemplate, parse } from 'vue/compiler-sfc'

// userTranslationKeys.spec.ts checks translation availability for user-facing
// pages and their local dependencies. Parsed module references select reachable
// source files, and parsed calls select literal keys from scripts and compiled
// Vue templates. Bilingual catalogs also share named interpolation inputs.

// SourceReferences records dependencies and literal translation keys from parsed source.
interface SourceReferences {
  imports: string[] // Referenced module specifiers.
  keys: string[] // Literal translation arguments.
  catalogPrefixes: string[] // Administrator key constants and dynamic key prefixes.
}

// sourceReferences extracts imports and literal translation calls from one source.
// Vue scripts and compiled template expressions share the TypeScript visitor, so
// quoted examples and comments remain data. Vue parse or compilation errors fail
// the test immediately so malformed components remain visible as failures.
// Administrator key strings and template prefixes also identify domains used by
// stored labels or dynamically selected fields, which literal call checks omit.
function sourceReferences(source: string, filename: string): SourceReferences {
  const result: SourceReferences = { imports: [], keys: [], catalogPrefixes: [] }
  const scripts: string[] = []
  if (filename.endsWith('.vue')) {
    const { descriptor, errors } = parse(source, { filename })
    if (errors.length) throw new Error(`Invalid Vue source ${filename}: ${errors.join(', ')}`)
    for (const block of [descriptor.script, descriptor.scriptSetup]) {
      if (block) scripts.push(block.content)
    }
    if (descriptor.template) {
      // Compile directives and interpolations before visiting calls, so Vue
      // expression syntax and plain template text retain their distinct meanings.
      const template = compileTemplate({
        source: descriptor.template.content,
        filename,
        id: filename,
        compilerOptions: { expressionPlugins: ['typescript'] },
      })
      if (template.errors.length)
        throw new Error(`Invalid Vue template ${filename}: ${template.errors.join(', ')}`)
      scripts.push(template.code)
    }
  } else {
    scripts.push(source)
  }

  // visit records module references and dotted literal keys in syntax order.
  // Identifier and property calls cover script helpers and Vue's generated
  // context accesses; dynamic arguments remain outside this literal-key check.
  function visit(node: ts.Node): void {
    const catalogText = ts.isStringLiteralLike(node)
      ? node.text
      : ts.isTemplateExpression(node)
        ? node.head.text
        : ''
    if (
      /^admin\.(providers|settings|ops|errorPassthrough|tlsFingerprintRouters|tlsFingerprintProfiles)(?:\.[\w]*)*$/.test(
        catalogText,
      )
    ) {
      result.catalogPrefixes.push(catalogText)
    }
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      result.imports.push(node.moduleSpecifier.text)
    }
    if (ts.isCallExpression(node)) {
      const argument = node.arguments[0]
      if (argument && ts.isStringLiteralLike(argument)) {
        if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
          result.imports.push(argument.text)
        } else {
          const name = ts.isIdentifier(node.expression)
            ? node.expression.text
            : ts.isPropertyAccessExpression(node.expression)
              ? node.expression.name.text
              : ''
          if ((name === 't' || name === '$t') && /^[A-Za-z]\w*(?:\.\w+)+$/.test(argument.text))
            result.keys.push(argument.text)
        }
      }
    }
    ts.forEachChild(node, visit)
  }
  for (const script of scripts)
    visit(ts.createSourceFile(filename, script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS))
  return result
}

// root locates frontend source files when the suite is launched from either the
// repository or frontend directory.
const root = existsSync(resolve(process.cwd(), 'src/views'))
  ? resolve(process.cwd(), 'src')
  : resolve(process.cwd(), 'frontend/src')
// filesUnder recursively lists TypeScript and Vue sources beneath the supplied
// directory. Test directories and test files are excluded so resource checks
// follow production consumers; filesystem errors propagate to the test.
function filesUnder(path: string): string[] {
  return readdirSync(path, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === '__tests__') return []
    const file = join(path, entry.name)
    return entry.isDirectory()
      ? filesUnder(file)
      : /\.(vue|ts)$/.test(file) && !/\.(spec|test)\.ts$/.test(file)
        ? [file]
        : []
  })
}

// sourceCache shares parsed production dependencies across page coverage checks.
// Tests parse each reachable file once while keeping fixture parsing independent.
const sourceCache = new Map<string, SourceReferences>()

// referencesForFile reads and parses one production file on its first visit.
// Later callers share the cached references and treat their arrays as immutable;
// file or parsing errors propagate to the coverage test.
function referencesForFile(file: string): SourceReferences {
  let references = sourceCache.get(file)
  if (!references) {
    references = sourceReferences(readFileSync(file, 'utf8'), file)
    sourceCache.set(file, references)
  }
  return references
}

// sourcesFrom follows local module references from the supplied entry files.
// Catalogs and router modules are excluded because they describe navigation and
// loading rather than rendered page dependencies. Dynamic components remain included
// so dialogs opened after navigation also have their translation resources checked.
function sourcesFrom(entries: string[]): string[] {
  const queue = [...entries]
  const visited = new Set<string>()
  while (queue.length) {
    const file = queue.pop()!
    if (visited.has(file) || file.includes('/i18n/locales/') || file.includes('/router/')) continue
    visited.add(file)
    for (const reference of referencesForFile(file).imports) {
      if (!reference.startsWith('@/') && !reference.startsWith('.')) continue
      const base = reference.startsWith('@/')
        ? join(root, reference.slice(2))
        : resolve(dirname(file), reference)
      // Resolve local module filenames before traversal so a shared dialog's imports
      // receive the same coverage as the page that opens it.
      const target = [base, `${base}.ts`, `${base}.vue`, join(base, 'index.ts')].find(
        (candidate) => /\.(vue|ts)$/.test(candidate) && existsSync(candidate),
      )
      if (target && target.startsWith(root)) queue.push(target)
    }
  }
  return [...visited].sort()
}

// userSources finds user-facing pages, the root shell, and reachable local modules.
// Shared translation editors are included because administrators can reuse their
// generic fields outside a domain configuration page.
function userSources(): string[] {
  return sourcesFrom([
    ...['views/user', 'views/public', 'views/auth'].flatMap((path) => filesUnder(join(root, path))),
    ...[
      'views/HomeView.vue',
      'views/ModelMarketplaceView.vue',
      'views/KeyUsageView.vue',
      'views/NotFoundView.vue',
      'components/common/LocalizedEditor.vue',
      'components/common/LocalizedFieldsEditor.vue',
      'App.vue',
    ].map((path) => join(root, path)),
  ])
}

// adminEntries reads actual administrator route paths and their lazy component targets.
// Redirect records have no component and are omitted. Parsing the production route
// table makes a newly added destination participate in resource coverage automatically.
function adminEntries(): Array<{ path: string; file: string }> {
  const filename = join(root, 'router/routes/admin.ts')
  const source = ts.createSourceFile(
    filename,
    readFileSync(filename, 'utf8'),
    ts.ScriptTarget.Latest,
    true,
  )
  const entries: Array<{ path: string; file: string }> = []
  // visit finds component-bearing route objects and resolves their application imports.
  function visit(node: ts.Node): void {
    if (ts.isObjectLiteralExpression(node)) {
      const properties = node.properties.filter(ts.isPropertyAssignment)
      const path = properties.find((property) => property.name.getText(source) === 'path')
      const component = properties.find((property) => property.name.getText(source) === 'component')
      if (path && ts.isStringLiteral(path.initializer) && component) {
        const target = sourceReferences(component.getText(source), filename).imports.find(
          (reference) => reference.startsWith('@/'),
        )
        if (target) entries.push({ path: path.initializer.text, file: join(root, target.slice(2)) })
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(source)
  return entries
}

// hasKey reports whether a dotted catalog path resolves to a string value.
// Missing segments and non-object intermediate values return false, allowing the
// caller to report unresolved translation calls.
function hasKey(value: unknown, key: string): boolean {
  let current = value
  for (const segment of key.split('.')) {
    if (!current || typeof current !== 'object' || !(segment in current)) return false
    current = (current as Record<string, unknown>)[segment]
  }
  return typeof current === 'string'
}

// flatStrings collects nested string values under dotted paths into the supplied
// result object. Optional arguments start a fresh root traversal, while
// recursive calls share the accumulator and skip non-string primitive values.
function flatStrings(
  value: unknown,
  prefix = '',
  result: Record<string, string> = {},
): Record<string, string> {
  if (typeof value === 'string') result[prefix] = value
  else if (value && typeof value === 'object') {
    for (const [key, item] of Object.entries(value))
      flatStrings(item, prefix ? `${prefix}.${key}` : key, result)
  }
  return result
}

describe('translation source parsing', () => {
  // Real module declarations and multiline calls appear beside quoted and
  // commented lookalikes. Collection retains executable references and literal
  // keys while excluding examples and dynamic translation arguments.
  it('collects TypeScript module references and calls from syntax', () => {
    const references = sourceReferences(
      `
      import value from '@/real/static'
      import '@/real/side-effect'
      export { value } from './real/export'
      const lazy = import('./real/dynamic')
      // import ignored from '@/fake/comment'; t('fake.comment')
      const example = "import ignored from '@/fake/string'; t('fake.string')"
      t /* formatting */ (
        'real.multiline'
      )
      i18n.t('real.member')
      $t('real.dollar')
      t(dynamicKey)
    `,
      'fixture.ts',
    )
    expect(references.imports).toEqual([
      '@/real/static',
      '@/real/side-effect',
      './real/export',
      './real/dynamic',
    ])
    expect(references.keys).toEqual(['real.multiline', 'real.member', 'real.dollar'])
  })

  // Vue directives and interpolations contain executable translations beside
  // literal attributes, text, and comments. Both script blocks and template
  // calls contribute keys while those literal examples stay excluded.
  it('collects Vue script and template calls while ignoring lookalikes', () => {
    const references = sourceReferences(
      `
      <script lang="ts">
        import regular from './real/regular'
        const label = t('real.regular')
      </script>
      <script setup lang="ts">
        import setup from './real/setup'
        const title = t('real.setup')
        const example = "t('fake.script')"
      </script>
      <template>
        <!-- t('fake.comment') import fake from './fake/comment' -->
        <div :title="t('real.title')" @click="$t('real.event')" title="t('fake.attribute')">
          {{ $t('real.interpolation') }}
          {{ "t('fake.quoted')" }}
          t('fake.text')
        </div>
      </template>
    `,
      'Fixture.vue',
    )
    expect(references.imports.filter((reference) => reference.startsWith('.'))).toEqual([
      './real/regular',
      './real/setup',
    ])
    expect(references.keys.sort()).toEqual([
      'real.event',
      'real.interpolation',
      'real.regular',
      'real.setup',
      'real.title',
    ])
  })
})

describe('user translation resources', () => {
  // User-facing pages and reachable local modules call translations with literal
  // keys. Both ordinary-page catalogs resolve every collected key to a string,
  // reporting missing paths with their source file.
  it('resolves literal keys referenced by user pages and their dependencies', () => {
    const missing = new Set<string>()
    for (const file of userSources()) {
      for (const key of referencesForFile(file).keys) {
        for (const [code, messages] of [
          ['en', enBase],
          ['zh-Hans', zhBase],
        ] as const) {
          if (!hasKey(messages, key)) missing.add(`${code}: ${key} (${file.slice(root.length)})`)
        }
      }
    }
    expect([...missing]).toEqual([])
  })

  // Shared badges, payment errors, and model controls use administrator-namespaced
  // constants and dynamic keys. Their reachable prefixes resolve in the ordinary
  // dictionary before any administrator page loads a larger domain.
  it('retains shared administrator labels and dynamic payment error fields', () => {
    const english = flatStrings(en)
    const missing = new Set<string>()
    for (const file of userSources()) {
      for (const prefix of referencesForFile(file).catalogPrefixes) {
        for (const key of Object.keys(english).filter((key) => key.startsWith(prefix))) {
          if (!hasKey(enBase, key) || !hasKey(zhBase, key)) missing.add(`${key}: ${file}`)
        }
      }
    }
    expect([...missing]).toEqual([])
    expect(enBase.admin.settings.payment).toMatchObject(
      Object.fromEntries(
        Object.entries(en.admin.settings.payment).filter(([key]) => key.startsWith('field_')),
      ),
    )
  })

  // English and Chinese catalogs describe the same interface with matching
  // interpolation inputs. Their flattened keys and named placeholder sets agree
  // so language changes retain required variables.
  it('keeps keys and named placeholders consistent', () => {
    const english = flatStrings(en)
    const chinese = flatStrings(zh)
    expect(Object.keys(english).sort()).toEqual(Object.keys(chinese).sort())
    // variables extracts unique named interpolation placeholders and returns
    // them in sorted order.
    const variables = (message: string) =>
      [...new Set([...message.matchAll(/\{([a-zA-Z_]\w*)\}/g)].map((match) => match[1]))].sort()
    const mismatches = Object.keys(english).filter(
      (key) => JSON.stringify(variables(english[key])) !== JSON.stringify(variables(chinese[key])),
    )
    expect(mismatches).toEqual([])
  })

  // The shared production manifest maps each language to a catalog directory.
  // Every mapped directory contains an entry file so the frontend can discover
  // the catalog for loading.
  it('has a loadable catalog for every production language', () => {
    for (const language of manifest.locales)
      expect(existsSync(join(root, 'i18n/locales', language.catalog, 'index.ts'))).toBe(true)
  })
})

describe('administrator route translation resources', () => {
  // A cold navigation prepares each real route and all statically reachable or lazy
  // dialogs. Literal keys, stored key strings, and dynamic administrator prefixes
  // must resolve before the page mounts in either supported language.
  it.each(adminEntries())('$path loads its reachable administrator domains', async (entry) => {
    vi.resetModules()
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'zh-Hans'
    await loadRouteLocaleMessages(entry.path)
    const prefixes = new Set(
      sourcesFrom([entry.file, join(root, 'App.vue')]).flatMap(
        (file) => referencesForFile(file).catalogPrefixes,
      ),
    )
    const missing: string[] = []
    for (const [locale, full] of [
      ['en', en],
      ['zh-Hans', zh],
    ] as const) {
      const messages = flatStrings(i18n.global.getLocaleMessage(locale))
      for (const [key, value] of Object.entries(flatStrings(full))) {
        if ([...prefixes].some((prefix) => key.startsWith(prefix)) && messages[key] !== value) {
          missing.push(`${locale}: ${key}`)
        }
      }
    }
    expect(missing).toEqual([])
  })
})
