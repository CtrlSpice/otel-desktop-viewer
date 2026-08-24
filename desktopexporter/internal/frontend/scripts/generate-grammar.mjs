// Regenerates the query-language parser from its grammar, then applies the
// type annotation the generated code needs to pass svelte-check: the
// specializer's `get` callback is emitted untyped, and this repo compiles
// generated code with the same strictness as written code.
//
// Run via: npm run generate:grammar
import { execSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'

const grammar = 'src/components/shared/Search/codemirror/query.grammar'
const out = 'src/components/shared/Search/codemirror/query.parser.ts'

execSync(`npx lezer-generator ${grammar} -o ${out}`, { stdio: 'inherit' })

let src = readFileSync(out, 'utf8')
src = src.replace(
  /get: \(value\) => (spec_\w+)\[value\] \|\| -1/g,
  'get: (value: string) => $1[value as keyof typeof $1] || -1'
)
writeFileSync(out, src)

execSync(`npx prettier --write ${out} ${out.replace('.ts', '.terms.ts')}`, {
  stdio: 'inherit',
})
console.log('grammar regenerated:', out)
