// Stamp the release tag into the packaging metadata. The Taskfile reads
// APP_VERSION for the Go binary, but the installers also carry the version in
// their own manifests, and a release must not advertise 0.1.0 forever.
import { readFileSync, writeFileSync } from 'node:fs'

const version = process.argv[2]
if (!version || !/^[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.+-]+)?$/.test(version)) {
  console.error(`usage: node build/stamp-version.mjs <x.y.z> (got ${version ?? ''})`)
  process.exit(1)
}

const files = [
  'build/config.yml',
  'build/darwin/Info.plist',
  'build/linux/nfpm/nfpm.yaml',
  'build/windows/info.json',
]

for (const file of files) {
  const text = readFileSync(file, 'utf8')
  if (!text.includes('0.1.0')) {
    console.error(`${file}: expected the 0.1.0 placeholder`)
    process.exit(1)
  }
  writeFileSync(file, text.replaceAll('0.1.0', version))
}