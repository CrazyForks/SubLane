import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { ApiError, request } from './request'
import { selectedWorkspace } from './workspace'

const settingsSchema = z.object({
  max_upload_bytes: z.number().int().positive(),
  restore_directory: z.string().min(1),
  destination_exists: z.boolean(),
})
const infoSchema = z.object({
  created_at: z.string().datetime({ offset: true }),
  version: z.string().min(1).max(128),
  schema_version: z.number().int().positive(),
  database_bytes: z.number().int().positive(),
})
export type BackupSettings = z.infer<typeof settingsSchema>
export type BackupInfo = z.infer<typeof infoSchema>
export type RestoredBackup = BackupInfo & { restore_directory: string }
export function backupOptions(client: QueryClient, userID: number) {
  return queryOptions({
    queryKey: ['backup-settings', userID],
    queryFn: ({ signal }) =>
      request('/api/settings/backup', settingsSchema, { signal }),
    enabled: () => {
      const user = client.getQueryData<AuthState>(authKey)?.user
      return user?.id === userID && user.role === 'admin'
    },
    staleTime: 0,
  })
}
export function verifyBackup(file: File, signal: AbortSignal) {
  return request('/api/settings/backup/verify', infoSchema, {
    method: 'POST',
    body: file,
    signal,
    headers: { 'Content-Type': 'application/gzip' },
  })
}
export function restoreBackup(file: File, signal: AbortSignal) {
  return request(
    '/api/settings/backup/restore',
    infoSchema.extend({ restore_directory: z.string().min(1) }),
    {
      method: 'POST',
      body: file,
      signal,
      headers: { 'Content-Type': 'application/gzip' },
    },
  )
}
export async function exportBackup(signal: AbortSignal) {
  const response = await fetch('/api/settings/backup/export', {
    method: 'POST',
    body: '{}',
    signal,
    credentials: 'same-origin',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/gzip',
      'X-SubLane-Workspace': String(selectedWorkspace()),
    },
  })
  if (!response.ok) {
    const value = z
      .object({ error: z.string() })
      .safeParse(await response.json().catch(() => null))
    throw new ApiError(
      value.success
        ? value.data.error
        : response.status === 401
          ? 'unauthorized'
          : 'backup_failed',
      response.status,
    )
  }
  if (response.headers.get('Content-Type') !== 'application/gzip')
    throw new ApiError('invalid_response', response.status)
  return response
}
export async function downloadBackup(
  response: Response,
  maxBytes: number,
  current: () => boolean,
) {
  const blob = await response.blob()
  if (!current()) return
  if (!blob.size || blob.size > maxBytes)
    throw new ApiError('backup_too_large', 413)
  const filename =
    response.headers
      .get('Content-Disposition')
      ?.match(
        /filename="(sublane-\d{8}-\d{6}\.sublane-backup\.tar\.gz)"/,
      )?.[1] ?? 'sublane.sublane-backup.tar.gz'
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  // Let the browser claim the download before releasing its short-lived blob URL.
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
export function backupErrorKey(error: Error) {
  if (error instanceof ApiError) {
    if (error.code === 'demo_read_only') return 'demoReadOnly'
    if (error.code === 'invalid_backup') return 'backupInvalid'
    if (error.code === 'backup_too_large') return 'backupTooLarge'
    if (error.code === 'backup_destination_exists')
      return 'backupDestinationExists'
    if (error.code === 'backup_audit_failed') return 'backupAuditFailed'
    if (error.code === 'backup_busy') return 'backupBusy'
  }
  return 'backupFailed'
}
