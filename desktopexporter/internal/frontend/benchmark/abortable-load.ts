export async function loadWithAbortTimeout<T>(
  load: (signal: AbortSignal) => Promise<T>,
  milliseconds: number,
  message: string
): Promise<T> {
  if (!Number.isSafeInteger(milliseconds) || milliseconds <= 0) {
    throw new TypeError('load timeout must be a positive safe integer')
  }

  const controller = new AbortController()
  const timeoutError = new Error(message)
  let timeoutID: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_resolve, reject) => {
    timeoutID = setTimeout(() => {
      controller.abort(timeoutError)
      reject(timeoutError)
    }, milliseconds)
  })

  try {
    return await Promise.race([load(controller.signal), timeout])
  } catch (error) {
    if (controller.signal.aborted && error !== timeoutError) {
      throw new Error(message, { cause: error })
    }
    throw error
  } finally {
    if (timeoutID !== undefined) clearTimeout(timeoutID)
  }
}
