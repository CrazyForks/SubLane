import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowUpRight, Check, Copy, LoaderCircle, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  accountErrorKey,
  beginAuthorization,
  cancelAuthorization,
  completeAuthorization,
  importAccount,
  type Account,
  type Provider,
  providers,
  providerLabels,
  callbackURLs,
} from '@/lib/accounts'
import { ProviderLogo } from './ProviderLogo'
import { ProxyPicker } from './ProxyPicker'
import { proxyOptions } from '@/lib/proxies'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Textarea } from './ui/Textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

type Props = {
  account?: Account
  onClose: () => void
  onCreated: () => Promise<void>
  restoreFocus: () => void
}

export function ConnectAccount({
  account,
  onClose,
  onCreated,
  restoreFocus,
}: Props) {
  const { t } = useTranslation()
  const [provider, setProvider] = useState<Provider>(
    account?.provider ?? 'codex',
  )
  const [method, setMethod] = useState<'oauth' | 'import'>('oauth')
  const [name, setName] = useState(account?.name ?? '')
  const [proxyID, setProxyID] = useState(account?.proxy_id ?? '')
  const proxies = useQuery({ ...proxyOptions, enabled: !account })
  const [authJSON, setAuthJSON] = useState('')
  const [callback, setCallback] = useState('')
  const [error, setError] = useState<
    'name' | 'json' | 'file' | 'size' | 'copy' | null
  >(null)
  const [copied, setCopied] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const authorizationLink = useRef<HTMLAnchorElement>(null)
  const pendingState = useRef<string | undefined>(undefined)
  const begin = useMutation({
    mutationFn: beginAuthorization,
    gcTime: 0,
    onSuccess: (value) => {
      pendingState.current = value.state
    },
  })
  const imported = useMutation({
    mutationFn: importAccount,
    gcTime: 0,
    onSuccess: async () => {
      setAuthJSON('')
      await onCreated()
      onClose()
    },
  })
  const finish = useMutation({
    mutationFn: completeAuthorization,
    gcTime: 0,
    onSuccess: async () => {
      pendingState.current = undefined
      setCallback('')
      await onCreated()
      onClose()
    },
  })
  const busy = begin.isPending || imported.isPending || finish.isPending
  useEffect(() => {
    if (begin.data) authorizationLink.current?.focus()
  }, [begin.data])
  useEffect(
    () => () => {
      // Closing the form releases credentials. Server expiry remains the fallback if cancellation cannot reach it.
      if (pendingState.current)
        cancelAuthorization(pendingState.current).catch(() => undefined)
    },
    [],
  )
  const close = () => {
    if (!busy) onClose()
  }
  const start = () => {
    setError(null)
    const value = name.trim()
    const characters = Array.from(value)
    if (
      characters.length < 1 ||
      characters.length > 64 ||
      characters.some(
        (character) =>
          character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127,
      )
    ) {
      setError('name')
      return
    }
    finish.reset()
    setCallback('')
    setCopied(false)
    begin.mutate({
      provider,
      name: value,
      replace_id: account?.id,
      proxy_id: proxyID || undefined,
    })
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (busy) return
    setError(null)
    if (method === 'oauth') {
      if (begin.data)
        finish.mutate({
          state: begin.data.state,
          callback_url: callback.trim(),
        })
      else start()
      return
    }
    const length = Array.from(name.trim()).length
    if (length < 1 || length > 64) {
      setError('name')
      return
    }
    try {
      const data: unknown = JSON.parse(authJSON)
      if (!data || typeof data !== 'object' || Array.isArray(data)) {
        setError('json')
        return
      }
    } catch {
      setError('json')
      return
    }
    if (new TextEncoder().encode(authJSON).length > 65536) {
      setError('size')
      return
    }
    imported.mutate({
      provider,
      name: name.trim(),
      auth_json: authJSON,
      replace_id: account?.id,
      proxy_id: proxyID || undefined,
    })
  }
  const readFile = async (file?: File) => {
    if (!file) return
    setError(null)
    setAuthJSON('')
    if (file.size > 65536) {
      setError('size')
      return
    }
    try {
      setAuthJSON(await file.text())
    } catch {
      setError('file')
    }
    if (fileInput.current) fileInput.current.value = ''
  }
  const copy = async () => {
    if (!begin.data) return
    try {
      await navigator.clipboard.writeText(begin.data.url)
      setCopied(true)
    } catch {
      setError('copy')
    }
  }
  const failure = finish.error ?? imported.error ?? begin.error
  const fieldErrors = {
    name: 'accountNameInvalid',
    json: 'accountJSONInvalid',
    file: 'fileReadFailed',
    size: 'fileTooLarge',
    copy: 'copyKeyFailed',
  } as const
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto [@media(pointer:coarse)]:[&_button]:min-h-11 [@media(pointer:coarse)]:[&_a]:min-h-11"
        onInteractOutside={(event) => {
          if (busy) event.preventDefault()
        }}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          restoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(account ? 'reauthorizeAccount' : 'connectAccountTitle')}
          </DialogTitle>
          <DialogDescription>
            {t(
              method === 'oauth'
                ? 'authorizationInstructions'
                : provider === 'codex'
                  ? 'accountImportDescription'
                  : 'providerImportDescription',
            )}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} noValidate className="space-y-5">
          <div className="space-y-2">
            <p id="provider-label" className="text-sm font-medium">
              {t('serviceProvider')}
            </p>
            <div
              role="group"
              aria-labelledby="provider-label"
              className="grid grid-cols-3 gap-2"
            >
              {providers.map((id) => (
                <Button
                  key={id}
                  type="button"
                  variant="ghost"
                  aria-label={providerLabels[id]}
                  aria-pressed={provider === id}
                  disabled={busy || Boolean(account) || Boolean(begin.data)}
                  className="relative h-auto min-w-0 flex-col gap-2 rounded-lg border border-border bg-card px-2 py-3 shadow-none hover:bg-muted aria-pressed:border-foreground aria-pressed:bg-muted dark:hover:bg-muted"
                  onClick={() => {
                    if (provider === id) return
                    setProvider(id)
                    setAuthJSON('')
                    setCallback('')
                    setError(null)
                    begin.reset()
                    imported.reset()
                  }}
                >
                  <ProviderLogo provider={id} />
                  <span className="text-xs leading-5">
                    {id === 'antigravity' ? 'Antigravity' : providerLabels[id]}
                  </span>
                  {provider === id && (
                    <Check
                      className="absolute top-1.5 right-1.5 size-3"
                      aria-hidden="true"
                    />
                  )}
                </Button>
              ))}
            </div>
          </div>
          <div className="space-y-2">
            <label htmlFor="account-name" className="text-sm font-medium">
              {t('accountName')}
            </label>
            <Input
              id="account-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              readOnly={Boolean(account) || Boolean(begin.data)}
              maxLength={128}
              disabled={busy}
              aria-invalid={error === 'name'}
            />
          </div>
          {!account && (
            <div className="space-y-2">
              {proxies.isPending ? (
                <p role="status" className="text-sm text-muted-foreground">
                  {t('loadingProxies')}
                </p>
              ) : proxies.isError ? (
                <p role="alert" className="text-sm text-error">
                  {t('proxiesLoadFailed')}
                </p>
              ) : (
                <ProxyPicker
                  value={proxyID}
                  onChange={setProxyID}
                  proxies={proxies.data.proxies}
                  disabled={busy || Boolean(begin.data)}
                />
              )}
              <p className="text-xs leading-5 text-muted-foreground">
                {t('accountProxyHint')}
              </p>
            </div>
          )}
          {!begin.data && (
            <div
              role="group"
              aria-label={t('connectionMethod')}
              className="flex flex-wrap gap-2"
            >
              <Button
                type="button"
                size="sm"
                variant={method === 'oauth' ? 'secondary' : 'ghost'}
                aria-pressed={method === 'oauth'}
                onClick={() => {
                  setMethod('oauth')
                  setError(null)
                  imported.reset()
                }}
                disabled={busy}
              >
                {t('accountOAuth')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant={method === 'import' ? 'secondary' : 'ghost'}
                aria-pressed={method === 'import'}
                onClick={() => {
                  setMethod('import')
                  setError(null)
                  begin.reset()
                }}
                disabled={busy}
              >
                {t('accountImport')}
              </Button>
            </div>
          )}
          {method === 'import' ? (
            <div className="space-y-3">
              <input
                ref={fileInput}
                type="file"
                accept=".json,application/json"
                aria-label={t('authJSONFile')}
                className="hidden"
                onChange={(event) => readFile(event.target.files?.[0])}
                disabled={busy}
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => fileInput.current?.click()}
                disabled={busy}
              >
                <Upload aria-hidden="true" />
                {t('chooseAuthFile')}
              </Button>
              <div className="space-y-2">
                <label htmlFor="auth-json" className="text-sm font-medium">
                  {t('authJSONContents')}
                </label>
                <Textarea
                  id="auth-json"
                  rows={6}
                  value={authJSON}
                  onChange={(event) => setAuthJSON(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={busy}
                  aria-invalid={error === 'json'}
                  className="resize-y font-mono"
                />
              </div>
            </div>
          ) : begin.data ? (
            <div className="space-y-5">
              <div className="flex flex-wrap gap-2">
                <Button asChild>
                  <a
                    ref={authorizationLink}
                    href={begin.data.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {t('openAuthorization')}
                    <ArrowUpRight aria-hidden="true" />
                  </a>
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t(copied ? 'copied' : 'copyAuthorization')}
                  title={t(copied ? 'copied' : 'copyAuthorization')}
                  onClick={copy}
                >
                  <Copy aria-hidden="true" />
                </Button>
              </div>
              <div className="space-y-2">
                <label htmlFor="oauth-callback" className="text-sm font-medium">
                  {t('callbackURL')}
                </label>
                <Textarea
                  id="oauth-callback"
                  placeholder={`${begin.data.callback_url ?? callbackURLs[provider]}?...`}
                  rows={3}
                  value={callback}
                  onChange={(event) => setCallback(event.target.value)}
                  maxLength={8192}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={busy}
                  aria-describedby="callback-hint"
                />
                <p
                  id="callback-hint"
                  className="text-sm leading-6 text-muted-foreground"
                >
                  {t('callbackHint')}
                </p>
              </div>
            </div>
          ) : null}
          {(error || failure) && (
            <p role="alert" className="text-sm leading-6 text-error">
              {t(error ? fieldErrors[error] : accountErrorKey(failure))}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={close}
              disabled={busy}
            >
              {t('cancel')}
            </Button>
            {finish.isError && (
              <Button
                type="button"
                variant="ghost"
                onClick={start}
                disabled={busy}
              >
                {t('startAgain')}
              </Button>
            )}
            <Button
              type="submit"
              disabled={
                busy ||
                (method === 'oauth' && Boolean(begin.data) && !callback.trim())
              }
            >
              {busy && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t(
                method === 'import'
                  ? 'importAccount'
                  : begin.data
                    ? 'completeAuthorization'
                    : 'startAuthorization',
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
