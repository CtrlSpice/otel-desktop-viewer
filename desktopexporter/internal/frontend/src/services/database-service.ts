import type { DatabaseList } from '@/types/api-types'
import { callRPC, named } from './telemetry-request'

export const databaseAPI = {
  listDatabases: async (signal?: AbortSignal): Promise<DatabaseList> =>
    callRPC<DatabaseList>('listDatabases', undefined, signal),

  openDatabase: async (
    name: string,
    signal?: AbortSignal
  ): Promise<DatabaseList> =>
    callRPC<DatabaseList>('openDatabase', named({ name }), signal),

  newDatabase: async (
    name: string,
    signal?: AbortSignal
  ): Promise<DatabaseList> =>
    callRPC<DatabaseList>('newDatabase', named({ name }), signal),
}
