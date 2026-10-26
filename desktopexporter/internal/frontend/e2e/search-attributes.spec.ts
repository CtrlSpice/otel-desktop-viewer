import { expect, test } from '@playwright/test'

for (const [signal, scope, method] of [
  ['logs', 'log', 'getLogAttributeDefinitions'],
  ['traces', 'span', 'getTraceAttributeDefinitions'],
  ['metrics', 'datapoint', 'getMetricAttributeDefinitions'],
]) {
  test(`${signal} attribute tokens preserve exact queries through editing and clipboard`, async ({
    page,
  }) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    const requests: unknown[] = []
    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: false } } })
    )
    await page.route('**/rpc', async route => {
      const { id, method: rpcMethod, params } = route.request().postDataJSON()
      if (params?.query) requests.push(params.query)
      const result =
        rpcMethod === method
          ? [
              { name: 'attempts', type: 'int64', attributeScope: scope },
              { name: 'attempts', type: 'int64', attributeScope: 'resource' },
              { name: 'attempts', type: 'string', attributeScope: scope },
            ]
          : rpcMethod === 'getStats'
            ? {
                traces: {
                  traceCount: 0,
                  spanCount: 0,
                  serviceCount: 0,
                  errorCount: 0,
                  lastReceived: null,
                },
                logs: { logCount: 0, errorCount: 0, lastReceived: null },
                metrics: {
                  metricCount: 0,
                  dataPointCount: 0,
                  lastReceived: null,
                },
                rejections: [],
              }
            : rpcMethod === 'searchLogPage'
              ? { logs: [], total: 0 }
              : []
      await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
    })
    const query = `attr(${scope}, "attempts", int64) >= 9007199254740993`
    const definitions = page.waitForResponse(
      response =>
        response.url().endsWith('/rpc') &&
        response.request().postDataJSON().method === method
    )
    await page.goto(`/${signal}?q=${encodeURIComponent(query)}`)
    await definitions
    const editor = page.locator('.cm-content[contenteditable="true"]')
    const token = editor.locator('.cm-attribute-token')
    await expect(token).toContainText('attempts')
    await expect(editor).not.toContainText('attr(')
    await expect
      .poll(() => requests)
      .toContainEqual(
        expect.objectContaining({
          query: {
            field: {
              name: 'attempts',
              type: 'int64',
              searchScope: 'attribute',
              attributeScope: scope,
            },
            fieldOperator: '>=',
            value: '9007199254740993',
          },
        })
      )

    await editor.focus()
    await page.keyboard.press('ControlOrMeta+a')
    const copied = await editor.evaluate(element => {
      const data = new DataTransfer()
      element.dispatchEvent(
        new ClipboardEvent('copy', {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        })
      )
      return data.getData('text/plain')
    })
    expect(copied).toBe(query)
    await page.keyboard.press('Home')
    await page.keyboard.press('ArrowRight')
    await page.keyboard.press('Backspace')
    await expect(token).toHaveCount(0)
    await expect(editor).toContainText('>= 9007199254740993')
    await page.keyboard.press('ControlOrMeta+z')
    await expect(token).toHaveCount(1)

    await token.focus()
    await page.keyboard.press('Enter')
    await page
      .getByRole('option')
      .filter({ hasText: 'Resource · Integer' })
      .click()
    await expect(token).toContainText('Resource · Integer')
    await expect(editor).toContainText('>= 9007199254740993')
    await page.keyboard.press('Escape')
    await editor.press('Enter')
    const changed = 'attr(resource, "attempts", int64) >= 9007199254740993'
    await expect
      .poll(() => new URL(page.url()).searchParams.get('q'))
      .toBe(changed)
    await page.reload()
    await expect(token).toContainText('Resource · Integer')

    await editor.focus()
    await page.keyboard.press('ControlOrMeta+a')
    const cut = await editor.evaluate(element => {
      const data = new DataTransfer()
      element.dispatchEvent(
        new ClipboardEvent('cut', {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        })
      )
      return data.getData('text/plain')
    })
    expect(cut).toBe(changed)
    await expect(token).toHaveCount(0)
    await editor.evaluate((element, text) => {
      const data = new DataTransfer()
      data.setData('text/plain', text)
      element.dispatchEvent(
        new ClipboardEvent('paste', {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        })
      )
    }, cut)
    await expect(token).toContainText('Resource · Integer')
    await expect(editor).toContainText('>= 9007199254740993')

    await page
      .getByRole('button', { name: 'Clear search', exact: true })
      .click()
    await editor.pressSequentially('attempts')
    await page
      .getByRole('option')
      .filter({ hasText: 'Resource · Integer' })
      .click()
    await expect(token).toContainText('attempts')
    await editor.pressSequentially('>= 3')
    await page.keyboard.press('Escape')
    await editor.press('Enter')
    await expect
      .poll(() => new URL(page.url()).searchParams.get('q'))
      .toBe('attr(resource, "attempts", int64) >= 3')

    const previousURL = page.url()
    const multiline = `attr(${scope}, "attempts", int64) >= 9007199254740993\r\n  and attr(${scope}, "message", string) = "first\r\n  second"`
    await page.keyboard.press('ControlOrMeta+a')
    await editor.evaluate((element, text) => {
      const data = new DataTransfer()
      data.setData('text/plain', text)
      element.dispatchEvent(
        new ClipboardEvent('paste', {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        })
      )
    }, multiline)
    await expect(token).toHaveCount(2)
    await expect(editor).toContainText('"first\\r\\n  second"')
    await expect(editor.locator('.cm-line')).toHaveCount(1)
    expect(page.url()).toBe(previousURL)

    // Typing after a paste has its own undo step; the paste replaces one selection.
    await editor.pressSequentially(' ')
    await page.keyboard.press('ControlOrMeta+z')
    await expect(token).toHaveCount(2)
    await page.keyboard.press('ControlOrMeta+z')
    await expect(token).toHaveCount(1)
    await expect(editor).toContainText('>= 3')
    await page.keyboard.press('ControlOrMeta+Shift+z')
    await expect(token).toHaveCount(2)
    await page.keyboard.press('Escape')
    await editor.press('Enter')
    await expect
      .poll(() => new URL(page.url()).searchParams.get('q'))
      .toBe(
        `attr(${scope}, "attempts", int64) >= 9007199254740993 and attr(${scope}, "message", string) = "first\\r\\n  second"`
      )
    await expect
      .poll(() => requests)
      .toContainEqual(
        expect.objectContaining({
          group: expect.objectContaining({
            children: expect.arrayContaining([
              expect.objectContaining({
                query: expect.objectContaining({ value: 'first\r\n  second' }),
              }),
            ]),
          }),
        })
      )
    expect(errors).toEqual([])
  })
}
