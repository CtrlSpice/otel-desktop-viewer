import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { aggregate } from './aggregate.ts'
import { render } from './render.ts'
import { showReview } from '../human-review.ts'
import { presentationAssets } from './assets.ts'
const require = createRequire(import.meta.url)
const { format } = require('../../../desktopexporter/internal/frontend/node_modules/prettier') as typeof import('../../../desktopexporter/internal/frontend/node_modules/prettier/index.d.ts')

export async function build(run: string, output: string, reviewDirectories: string[] = []) {
  const read = (name: string, optional = false) => {
    const file = path.join(run, name)
    return optional && !fs.existsSync(file)
      ? undefined
      : (JSON.parse(fs.readFileSync(file, 'utf8')) as unknown)
  }
  const reviews = reviewDirectories.map(directory => {
    const review = showReview(directory)
    if (fs.realpathSync(review.run) !== fs.realpathSync(run))
      throw new Error('Human review belongs to a different run')
    return {
      model: review.investigator.model,
      task: review.taskID,
      sessionIDs: review.investigator.sessionIDs,
      score: review.finalScore,
      signedOff: review.signedOff,
      claimCounts: review.claimCounts,
    }
  })
  const data = aggregate(
    read('run-manifest.json'),
    read('pilot.json'),
    read('model-summary.json', true),
    read('grader-summary.json', true),
    read('pricing-snapshot.json', true),
    reviews
  )
  // Never serialize raw run data into the reader-facing artifact.
  const assets = presentationAssets()
  const assetDirectory = output + '.assets'
  const html = await format(render(data, './' + encodeURIComponent(path.basename(assetDirectory))), { parser: 'html' })
  const file = fs.openSync(output, 'wx', 0o600)
  let createdAssets = false
  try {
    fs.mkdirSync(assetDirectory, { mode: 0o700 })
    createdAssets = true
    for (const [name, asset] of assets) {
      fs.writeFileSync(path.join(assetDirectory, name), asset.body, { flag: 'wx', mode: 0o600 })
    }
    fs.writeFileSync(file, html)
  } catch (error) {
    if (createdAssets) fs.rmSync(assetDirectory, { recursive: true })
    fs.unlinkSync(output)
    throw error
  } finally {
    fs.closeSync(file)
  }
  return data
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === import.meta.filename
) {
  const [run, output, ...reviews] = process.argv.slice(2)
  if (!run || !output)
    throw new Error(
      'Usage: node presentation/build.ts RUN_DIRECTORY OUTPUT.html [REVIEW_DIRECTORY ...]'
    )
  const data = await build(run, output, reviews)
  console.log(
    `Presentation generated: ${data.models.length} models, ${data.attempts} recorded attempts. ${data.provisional ? 'Signoff pending.' : 'Recorded attempts signed off.'}`
  )
}
