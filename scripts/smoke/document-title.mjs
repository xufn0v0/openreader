import { readFileSync } from 'node:fs'

const indexHTML = new URL('../../frontend/index.html', import.meta.url)

// Contracts read the shell title from frontend/index.html instead of hard-coding a product name,
// so the tab-title product-name decision (D1) only has to change index.html.
export function readShellDocumentTitle() {
  const match = readFileSync(indexHTML, 'utf8').match(/<title>([^<]*)<\/title>/i)
  const title = match?.[1]?.trim()
  if (!title) throw new Error(`${indexHTML.pathname} must declare a non-empty <title>`)
  return title
}

export async function waitForDocumentTitle(page, expected, label, timeout = 10_000) {
  try {
    await page.waitForFunction(value => document.title === value, expected, { timeout })
  } catch (error) {
    const actual = await page.evaluate(() => document.title).catch(() => '<unavailable>')
    throw new Error(`${label}: tab title should be ${JSON.stringify(expected)}, got ${JSON.stringify(actual)} (${error.message.split('\n')[0]})`)
  }
}
