export type EvalRequestPart<T> =
  { status: 'pending' } | { status: 'ready'; value: T } | { status: 'failed' }

export type EvalRequestStatus = EvalRequestPart<unknown>['status']
