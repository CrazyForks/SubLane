import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { authKey, authOptions, type AuthState } from '@/lib/auth'
import {
  formatInstanceDate,
  saveTimeZone,
  timeZoneErrorKey,
  validTimeZone,
} from '@/lib/timezone'
import { Button } from './ui/Button'
import { Input } from './ui/Input'

export function TimeZoneSettings({ userID }: { userID: number }) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const { data } = useQuery(authOptions())
  const current = data?.time_zone ?? 'UTC'
  const [name, setName] = useState(current)
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState(false)
  const [previewAt, setPreviewAt] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setPreviewAt(Date.now()), 60_000)
    return () => window.clearInterval(timer)
  }, [])
  const mutation = useAdminMutation({
    userID,
    mutationFn: saveTimeZone,
    onSuccess: async (result) => {
      client.setQueryData<AuthState>(authKey, (state) =>
        state ? { ...state, time_zone: result.time_zone } : state,
      )
      setName(result.time_zone)
      setSaved(true)
      await Promise.all([
        client.invalidateQueries({ queryKey: ['allocations'] }),
        client.invalidateQueries({ queryKey: ['allocation'] }),
        client.invalidateQueries({ queryKey: ['own-allocations'] }),
      ])
    },
  })
  const change = (value: string) => {
    setName(value)
    setInvalid(false)
    setSaved(false)
    mutation.reset()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    if (!validTimeZone(name)) {
      setInvalid(true)
      return
    }
    mutation.mutate(name)
  }
  const preview = validTimeZone(name)
    ? formatInstanceDate(previewAt, i18n.resolvedLanguage ?? 'en', name, {
        dateStyle: 'full',
        timeStyle: 'medium',
      })
    : null
  return (
    <section
      className="mt-10 border-t border-border pt-6"
      aria-labelledby="timezone-title"
    >
      <h2 id="timezone-title" className="font-medium">
        {t('timeZoneTitle')}
      </h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t('timeZoneDescription')}
      </p>
      <form className="mt-6 max-w-md space-y-4" onSubmit={submit} noValidate>
        <div className="space-y-2">
          <label htmlFor="instance-timezone" className="text-sm font-medium">
            {t('timeZoneLabel')}
          </label>
          <Input
            id="instance-timezone"
            list="instance-timezone-options"
            value={name}
            onChange={(event) => change(event.target.value)}
            maxLength={64}
            autoComplete="off"
            spellCheck={false}
            disabled={mutation.isPending}
            aria-invalid={invalid}
            aria-describedby="instance-timezone-hint"
          />
          <datalist id="instance-timezone-options">
            {[
              'UTC',
              'Asia/Shanghai',
              'Asia/Tokyo',
              'Asia/Singapore',
              'Europe/London',
              'Europe/Berlin',
              'America/New_York',
              'America/Los_Angeles',
            ].map((zone) => (
              <option key={zone} value={zone} />
            ))}
          </datalist>
          <p
            id="instance-timezone-hint"
            className="text-xs leading-5 text-muted-foreground"
          >
            {t('timeZoneHint')}
          </p>
        </div>
        {preview && (
          <p className="text-sm text-muted-foreground">
            {t('timeZonePreview', { time: preview, zone: name })}
          </p>
        )}
        {invalid && (
          <p role="alert" className="text-sm text-error">
            {t('timeZoneInvalid')}
          </p>
        )}
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t(timeZoneErrorKey(mutation.error))}
          </p>
        )}
        {saved && (
          <p role="status" className="text-sm text-success">
            {t('timeZoneSaved')}
          </p>
        )}
        <Button type="submit" disabled={mutation.isPending || name === current}>
          {t('timeZoneSave')}
        </Button>
      </form>
    </section>
  )
}
