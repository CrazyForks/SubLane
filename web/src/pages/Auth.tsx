import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, LoaderCircle } from 'lucide-react'
import { authKey, authOptions, setup, signIn } from '@/lib/auth'
import { ApiError } from '@/lib/request'
import { replaceAuthState } from '@/lib/query'
import { validateCredentials, type CredentialError } from '@/lib/credentials'
import { AuthShell } from '@/components/AuthShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

const messages = {
  invalid_credentials: 'invalidCredentials',
  rate_limited: 'authRateLimited',
  auth_busy: 'authBusy',
  origin_rejected: 'authOriginError',
  invalid_input: 'authInvalidInput',
  already_initialized: 'setupClosed',
} as const

export function Auth({ mode }: { mode: 'setup' | 'login' }) {
  const creating = mode === 'setup'
  const [setupStep, setSetupStep] = useState<'account' | 'workspace'>('account')
  const { t } = useTranslation()
  const client = useQueryClient()
  const { data: state } = useQuery(authOptions())
  const demo = !creating ? state?.demo : undefined
  const navigate = useNavigate()
  const heading = useRef<HTMLHeadingElement>(null)
  const [fieldError, setFieldError] = useState<CredentialError | null>(null)
  useEffect(() => {
    if (creating) heading.current?.focus()
  }, [creating, setupStep])
  const mutation = useMutation({
    meta: { demoAllowed: !creating },
    // Drop submitted credentials when the authentication form is no longer observed.
    gcTime: 0,
    mutationFn: (input: {
      username: string
      password: string
      workspace_name?: string
    }) =>
      creating
        ? setup({
            username: input.username,
            password: input.password,
            workspace_name: input.workspace_name ?? '',
          })
        : signIn({ username: input.username, password: input.password }),
    onSuccess: (state) => replaceAuthState(client, state),
    onError: (error) => {
      if (error instanceof ApiError && error.code === 'already_initialized')
        return client.invalidateQueries({ queryKey: authKey })
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const form = event.currentTarget
    const data = new FormData(form)
    const input = {
      username: String(data.get('username') ?? '').trim(),
      password: String(data.get('password') ?? ''),
    }
    setFieldError(null)
    mutation.reset()
    if (!creating || setupStep === 'account') {
      const validation = validateCredentials(
        input,
        creating ? String(data.get('confirm') ?? '') : undefined,
      )
      if (validation) {
        setFieldError(validation)
        const field = form.elements.namedItem(validation.field)
        if (field instanceof HTMLInputElement) field.focus()
        return
      }
      if (creating) {
        setSetupStep('workspace')
        return
      }
    }
    const workspaceName = String(data.get('workspace_name') ?? '').trim()
    if (
      creating &&
      (!workspaceName ||
        [...workspaceName].length > 64 ||
        /\p{Cc}/u.test(workspaceName))
    ) {
      setFieldError({
        field: 'workspace_name',
        message: 'workspaceNameInvalid',
      })
      const field = form.elements.namedItem('workspace_name')
      if (field instanceof HTMLInputElement) field.focus()
      return
    }
    mutation.mutate(
      creating ? { ...input, workspace_name: workspaceName } : input,
    )
  }
  const errorCode =
    mutation.error instanceof ApiError ? mutation.error.code : 'unavailable'
  const errorKey = Object.hasOwn(messages, errorCode)
    ? messages[errorCode as keyof typeof messages]
    : 'unavailable'
  const field = (
    name: string,
    label: 'username' | 'password' | 'confirmPassword',
    autoComplete: string,
    hint?: 'usernameHint' | 'passwordHint',
  ) => (
    <div className="space-y-2">
      <label htmlFor={'auth-' + name} className="text-sm font-medium">
        {t(label)}
      </label>
      <Input
        id={'auth-' + name}
        name={name}
        type={name === 'username' ? 'text' : 'password'}
        autoComplete={autoComplete}
        autoCapitalize="none"
        spellCheck={false}
        maxLength={name === 'username' ? 32 : 256}
        required
        disabled={mutation.isPending}
        aria-invalid={fieldError?.field === name}
        aria-describedby={
          fieldError?.field === name
            ? 'field-error'
            : hint
              ? 'hint-' + name
              : undefined
        }
      />
      {fieldError?.field === name ? (
        <p id="field-error" role="alert" className="text-sm text-error">
          {t(fieldError.message)}
        </p>
      ) : hint ? (
        <p
          id={'hint-' + name}
          className="text-xs leading-5 text-muted-foreground"
        >
          {t(hint)}
        </p>
      ) : null}
    </div>
  )
  return (
    <AuthShell>
      {creating && (
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2.5 mb-6 text-muted-foreground"
          disabled={mutation.isPending}
          onClick={() => {
            if (setupStep === 'workspace') {
              setFieldError(null)
              setSetupStep('account')
            } else {
              navigate({ to: '/setup' })
            }
          }}
        >
          <ArrowLeft aria-hidden="true" />
          {t(setupStep === 'workspace' ? 'setupBackToAdmin' : 'backToWelcome')}
        </Button>
      )}
      {creating && (
        <p className="mb-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t('setupProgress', { current: setupStep === 'account' ? 1 : 2 })}
        </p>
      )}
      <h1
        ref={heading}
        tabIndex={-1}
        className="text-2xl font-semibold tracking-tight outline-none"
      >
        {t(
          creating
            ? setupStep === 'account'
              ? 'createAdministrator'
              : 'setupFirstWorkspace'
            : 'signInTitle',
        )}
      </h1>
      {demo && (
        <div className="mt-5 space-y-3 rounded-lg border border-border bg-muted/50 p-4">
          <p className="text-sm font-medium">{t('demoTitle')}</p>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('demoDescription')}
          </p>
          <p className="break-words text-sm">{t('demoCredentials', demo)}</p>
          <Button
            type="button"
            variant="outline"
            className="w-full"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate(demo)}
          >
            {t('demoExplore')}
          </Button>
        </div>
      )}
      {creating && (
        <p className="mb-7 mt-2 text-sm leading-6 text-muted-foreground">
          {t(
            setupStep === 'account'
              ? 'setupDescription'
              : 'setupFirstWorkspaceDescription',
          )}
        </p>
      )}
      <form
        noValidate
        onSubmit={submit}
        className={creating ? 'space-y-5' : 'mt-7 space-y-5'}
      >
        <div
          hidden={creating && setupStep === 'workspace'}
          className="space-y-5"
        >
          {field(
            'username',
            'username',
            'username',
            creating ? 'usernameHint' : undefined,
          )}
          {field(
            'password',
            'password',
            creating ? 'new-password' : 'current-password',
            creating ? 'passwordHint' : undefined,
          )}
          {creating && field('confirm', 'confirmPassword', 'new-password')}
        </div>
        {creating && (
          <div hidden={setupStep !== 'workspace'} className="space-y-2">
            <label
              htmlFor="auth-workspace-name"
              className="text-sm font-medium"
            >
              {t('workspaceName')}
            </label>
            <Input
              id="auth-workspace-name"
              name="workspace_name"
              autoComplete="organization"
              maxLength={64}
              required
              disabled={mutation.isPending}
              aria-invalid={fieldError?.field === 'workspace_name'}
              aria-describedby={
                fieldError?.field === 'workspace_name'
                  ? 'field-error'
                  : undefined
              }
            />
            {fieldError?.field === 'workspace_name' && (
              <p id="field-error" role="alert" className="text-sm text-error">
                {t(fieldError.message)}
              </p>
            )}
          </div>
        )}
        {mutation.isError && (
          <p role="alert" className="text-sm leading-6 text-error">
            {t(errorKey, {
              seconds:
                mutation.error instanceof ApiError
                  ? mutation.error.retryAfter || 60
                  : 60,
            })}
          </p>
        )}
        <Button type="submit" className="w-full" disabled={mutation.isPending}>
          {mutation.isPending && (
            <LoaderCircle
              className="motion-safe:animate-spin"
              aria-hidden="true"
            />
          )}
          {t(
            mutation.isPending
              ? creating
                ? 'setupFinishing'
                : 'signingIn'
              : creating
                ? setupStep === 'account'
                  ? 'setupContinue'
                  : 'setupFinish'
                : 'signIn',
          )}
        </Button>
      </form>
    </AuthShell>
  )
}
