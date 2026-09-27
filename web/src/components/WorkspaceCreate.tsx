import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createWorkspace } from '@/lib/tenants'
import { ApiError } from '@/lib/request'
import { selectWorkspace } from '@/lib/workspace'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from './ui/Sheet'

export function WorkspaceCreate({
  onCreated,
  open: controlledOpen,
  onOpenChange,
}: {
  onCreated?: (id: number) => void
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [localOpen, setLocalOpen] = useState(false)
  const open = controlledOpen ?? localOpen
  const setOpen = onOpenChange ?? setLocalOpen
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: createWorkspace,
    onSuccess: (workspace) => {
      setOpen(false)
      if (onCreated) {
        onCreated(workspace.id)
      } else {
        selectWorkspace(workspace.id)
        window.location.assign('/')
      }
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = String(
      new FormData(event.currentTarget).get('name') ?? '',
    ).trim()
    const bad = !name || [...name].length > 64
    setInvalid(bad)
    if (bad || mutation.isPending) return
    mutation.mutate({ name })
  }
  return (
    <>
      {controlledOpen === undefined && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setOpen(true)}
        >
          {t('workspaceCreate')}
        </Button>
      )}
      <Sheet
        open={open}
        onOpenChange={(value) => {
          if (!mutation.isPending) setOpen(value)
        }}
      >
        <SheetContent
          className="w-full sm:max-w-md"
          closeDisabled={mutation.isPending}
        >
          <SheetHeader>
            <SheetTitle>{t('workspaceCreate')}</SheetTitle>
            <SheetDescription>
              {t('workspaceCreateDescription')}
            </SheetDescription>
          </SheetHeader>
          <form onSubmit={submit} className="space-y-5 px-4 pb-6" noValidate>
            <div className="space-y-2">
              <label htmlFor="workspace-name" className="text-sm font-medium">
                {t('workspaceName')}
              </label>
              <Input
                id="workspace-name"
                name="name"
                required
                maxLength={64}
                autoFocus
                disabled={mutation.isPending}
                aria-invalid={invalid}
              />
              {invalid && (
                <p role="alert" className="text-sm text-error">
                  {t('workspaceNameInvalid')}
                </p>
              )}
            </div>
            {mutation.isError && (
              <p role="alert" className="text-sm text-error">
                {mutation.error instanceof ApiError &&
                mutation.error.code === 'workspace_limit_reached'
                  ? t('workspaceLimitReached')
                  : t('workspaceCreateFailed')}
              </p>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {t('workspaceSave')}
            </Button>
          </form>
        </SheetContent>
      </Sheet>
    </>
  )
}
