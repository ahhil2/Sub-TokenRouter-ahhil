// build.mjs compiles the portal login and register JSX sources into plain JavaScript
// and writes the result into the matching static pages under ../site. The pages load
// React, ReactDOM and lucide-react as pinned CDN globals, so compiling ahead of time
// removes the in-browser Babel compiler that every visitor previously downloaded and
// ran before the form could render.
//
// Each page holds an empty-or-generated <script data-portal-app="NAME"> element; the
// build replaces only that element's body and leaves the surrounding markup, styles
// and CDN tags untouched. Run it with `node deploy/superahhil/portal/build.mjs` from
// the repository root after editing a .jsx file, and commit the regenerated HTML.

import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repository = join(here, '..', '..', '..')
// Reuse the TypeScript compiler installed for the frontend instead of adding a
// separate toolchain for two static pages.
const ts = createRequire(join(repository, 'frontend', 'package.json'))('typescript')

// pages lists each JSX source compiled into the static page of the same name.
const pages = ['login', 'register']

// compile transpiles one JSX source into classic-runtime JavaScript that calls the
// global React.createElement. It throws when TypeScript reports a syntax error so a
// broken source never replaces a working page.
function compile(name) {
  const source = readFileSync(join(here, `${name}.jsx`), 'utf8')
  const result = ts.transpileModule(source, {
    fileName: `${name}.jsx`,
    reportDiagnostics: true,
    compilerOptions: {
      jsx: ts.JsxEmit.React,
      target: ts.ScriptTarget.ES2020,
      removeComments: false,
    },
  })
  if (result.diagnostics?.length) {
    const messages = result.diagnostics.map((item) =>
      ts.flattenDiagnosticMessageText(item.messageText, '\n'),
    )
    throw new Error(`${name}.jsx: ${messages.join('; ')}`)
  }
  // An inline script ends at the first closing script tag, so generated code must
  // not contain one.
  if (/<\/script/i.test(result.outputText)) {
    throw new Error(`${name}.jsx: compiled output contains a closing script tag`)
  }
  return result.outputText.trimEnd()
}

// writePage replaces the body of the page's portal script element with the compiled
// code, indented to match the surrounding markup. It throws when the page does not
// contain exactly one matching element.
function writePage(name, code) {
  const path = join(here, '..', 'site', `${name}.html`)
  const html = readFileSync(path, 'utf8')
  const pattern = new RegExp(
    `(    <script data-portal-app="${name}">\\n)[\\s\\S]*?(    </script>\\n)`,
    'g',
  )
  const matches = html.match(pattern)
  if (!matches || matches.length !== 1) {
    throw new Error(`${name}.html: expected one data-portal-app="${name}" script`)
  }
  const body = code
    .split('\n')
    .map((line) => (line ? `      ${line}` : line))
    .join('\n')
  writeFileSync(path, html.replace(pattern, (_, open, close) => `${open}${body}\n${close}`))
}

for (const name of pages) {
  writePage(name, compile(name))
  console.log(`built ${name}.html`)
}
