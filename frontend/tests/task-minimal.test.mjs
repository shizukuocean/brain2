import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const app = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
const main = await readFile(new URL('../src/main.ts', import.meta.url), 'utf8')
const css = await readFile(new URL('../src/task-minimal.css', import.meta.url), 'utf8')

test('home keeps one task surface and moves supporting information behind disclosures', () => {
  assert.match(app, /class="home-focus"/)
  assert.match(app, /class="home-progress"/)
  assert.match(app, /class="home-disclosures"/)
  assert.match(app, /<details class="home-commands">/)
  assert.match(app, /<details class="home-summary">/)
  assert.doesNotMatch(app, /<details class="home-(?:commands|summary)" open>/)
  assert.match(app, /class="home-disclosure-mark" aria-hidden="true"/)
  assert.doesNotMatch(app, /需要时展开，再复制给 Agent/)
  assert.doesNotMatch(app, /连接、项目与检查状态/)
  assert.match(app, /class="home-summary-actions"><button class="secondary"/)
})

test('task-minimal layer loads after the legacy stylesheet', () => {
  assert.ok(main.indexOf("import './style.css'") < main.indexOf("import './task-minimal.css'"))
  assert.match(css, /@import "\.\/tokens\.css";/)
  assert.match(css, /"focus progress"\s*"disclosures disclosures"/)
  assert.match(css, /\.home-disclosures details \{\s*grid-area: auto;/)
  assert.match(css, /\.home-step-list li:not\(\.current\) small/)
})

test('small windows reflow agent cards and Obsidian starts with detection', () => {
  assert.match(css, /@media \(max-width: 88rem\)[\s\S]*?\.agent-grid \{\s*grid-template-columns: minmax\(0, 1fr\);/)
  assert.match(css, /\.agent-card \{\s*grid-template-rows: auto;/)
  assert.match(app, /@click="detectObsidianInstallation"/)
  assert.match(app, /检测 Obsidian/)
  assert.match(app, /v-if="obsidianDetectionAttempted" class="text-button"/)
})

test('task pages omit explanations that repeat visible state or action semantics', () => {
  assert.doesNotMatch(app, /技术字段由 Agent 调查，回执通过后才登记/)
  assert.doesNotMatch(app, /断链、路径和身份一致性是事实；孤立文件只标记为候选，不自动修复/)
  assert.doesNotMatch(app, /运行后将列出必需文件、项目身份、路径一致性、断链、短链歧义和孤立候选/)
  assert.match(css, /\.health-summary \{\s*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\);/)
})
