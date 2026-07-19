import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(scriptDir, '..')
const sourceDir = resolve(frontendDir, '.output', 'public')
const assetsDir = resolve(frontendDir, '..', 'backend', 'desktop', 'assets')
const gitkeep = resolve(assetsDir, '.gitkeep')

if (!existsSync(sourceDir)) {
  throw new Error(`Nuxt public output not found: ${sourceDir}`)
}

mkdirSync(assetsDir, { recursive: true })

for (const entry of readdirSync(assetsDir)) {
  if (entry !== '.gitkeep') {
    rmSync(resolve(assetsDir, entry), { recursive: true, force: true })
  }
}

cpSync(sourceDir, assetsDir, {
  recursive: true,
  force: true,
  filter: (source) => source !== gitkeep,
})
