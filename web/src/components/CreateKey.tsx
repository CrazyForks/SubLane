import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Copy, LoaderCircle, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { availableGroupOptions } from '@/lib/groups'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import { createKey } from '@/lib/keys'
import { KeyExpiry } from './KeyExpiry'
import { expiryValue, type ExpiryChoice } from '@/lib/keys'
import { ApiError } from '@/lib/request'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'

export function CreateKey({
  userID,
  onCreated,
}: {
  userID: number
  onCreated: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button data-tour="key">
          <Plus aria-hidden="true" />
          {t('createKey')}
        </Button>
      </DialogTrigger>
      {open && (
        <KeyForm
          userID={userID}
          onClose={() => setOpen(false)}
          onCreated={onCreated}
        />
      )}
    </Dialog>
  )
}

function KeyForm({
  userID,
  onClose,
  onCreated,
}: {
  userID: number
  onClose: () => void
  onCreated: () => Promise<void>
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const groups = useQuery(availableGroupOptions(client, userID))
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const selectedGroup = groups.data?.groups.find(
    (group) => group.id === selectedID,
  )
  const [expiry, setExpiry] = useState<ExpiryChoice>('never')
  const [nameError, setNameError] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copyFailed, setCopyFailed] = useState(false)
  // Creation holds the secret only in this dialog; later copies use a separate owner-checked request.
  const mutation = useMutation({
    mutationFn: createKey,
    gcTime: 0,
    onSuccess: onCreated,
    onError: async (error) => {
      if (error instanceof ApiError && error.code === 'group_unavailable')
        await groups.refetch()
    },
  })
  const close = () => {
    mutation.reset()
    onClose()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending || groups.isError || !selectedGroup) return
    const data = new FormData(event.currentTarget)
    const name = String(data.get('name') ?? '').trim()
    const characters = Array.from(name)
    const invalid =
      characters.length < 1 ||
      characters.length > 64 ||
      characters.some((character) => {
        const code = character.charCodeAt(0)
        return code < 32 || (code >= 127 && code <= 159)
      })
    setNameError(invalid)
    if (invalid) return
    setSelectedID(selectedGroup.id)
    mutation.mutate({
      name,
      ...(selectedGroup.scheme_id
        ? { scheme_id: selectedGroup.scheme_id }
        : {}),
      group_id: selectedGroup.id,
      expires_at: expiryValue(expiry, null),
    })
  }
  const copy = async () => {
    if (!mutation.data) return
    try {
      await navigator.clipboard.writeText(mutation.data.secret)
      setCopied(true)
      setCopyFailed(false)
    } catch {
      setCopyFailed(true)
    }
  }
  return (
    <DialogContent
      showCloseButton={false}
      className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      onInteractOutside={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
      onEscapeKeyDown={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
    >
      <DialogHeader>
        <DialogTitle>
          {t(mutation.data ? 'saveKeyTitle' : 'createKeyTitle')}
        </DialogTitle>
        <DialogDescription>
          {t(mutation.data ? 'saveKeyDescription' : 'createKeyDescription')}
        </DialogDescription>
      </DialogHeader>
      {mutation.data ? (
        <>
          <Input
            readOnly
            value={mutation.data.secret}
            aria-label={t('apiKey')}
            autoComplete="off"
            className="font-mono text-sm"
            onFocus={(event) => event.currentTarget.select()}
          />
          {copyFailed && (
            <p role="alert" className="text-sm text-error">
              {t('copyKeyFailed')}
            </p>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => copy()}>
              <Copy aria-hidden="true" />
              {t(copied ? 'copied' : 'copyKey')}
            </Button>
            <Button onClick={close}>{t('done')}</Button>
          </DialogFooter>
        </>
      ) : (
        <form onSubmit={submit} noValidate className="space-y-5">
          <div className="space-y-2">
            <label htmlFor="key-name" className="text-sm font-medium">
              {t('keyName')}
            </label>
            <Input
              id="key-name"
              name="name"
              maxLength={128}
              autoComplete="off"
              required
              disabled={mutation.isPending}
              aria-invalid={nameError}
              aria-describedby={nameError ? 'key-name-error' : undefined}
            />
            {nameError && (
              <p
                id="key-name-error"
                role="alert"
                className="text-sm text-error"
              >
                {t('keyNameHint')}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium" id="key-group-label">
              {t('keyGroup')}
            </p>
            {groups.isPending ? (
              <p role="status" className="text-sm text-muted-foreground">
                {t('loadingGroups')}
              </p>
            ) : groups.isError ? (
              <div role="alert" className="space-y-2">
                <p className="text-sm text-error">{t('groupsLoadFailed')}</p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => groups.refetch()}
                >
                  {t('reconnect')}
                </Button>
              </div>
            ) : groups.data.groups.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('noAvailableGroups')}
              </p>
            ) : (
              <DropdownMenu modal={false}>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full justify-between"
                    aria-label={t('keyGroup')}
                    disabled={mutation.isPending}
                  >
                    <span className="truncate">
                      {selectedGroup
                        ? selectedGroup.scheme_id
                          ? t('allocationKeyChoice', {
                              scheme: selectedGroup.scheme_name,
                              pool: selectedGroup.name,
                            })
                          : selectedGroup.name
                        : t('chooseGroup')}
                    </span>
                    <ChevronDown aria-hidden="true" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-(--radix-dropdown-menu-trigger-width)">
                  <DropdownMenuRadioGroup
                    value={selectedGroup ? String(selectedGroup.id) : ''}
                    onValueChange={(value) => setSelectedID(Number(value))}
                  >
                    {groups.data.groups.map((group) => (
                      <DropdownMenuRadioItem
                        key={group.id}
                        value={String(group.id)}
                      >
                        <span className="min-w-0 flex-1 truncate">
                          {group.scheme_id
                            ? t('allocationKeyChoice', {
                                scheme: group.scheme_name,
                                pool: group.name,
                              })
                            : group.name}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {t('poolAccountCount', {
                            count: group.account_count,
                          })}
                        </span>
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            <p className="text-xs leading-5 text-muted-foreground">
              {t('keyGroupHint')}
            </p>
            {selectedGroup?.account_count === 0 && (
              <p role="status" className="text-sm leading-6 text-warning">
                {t('keyEmptyPoolWarning')}
              </p>
            )}
          </div>
          <KeyExpiry
            value={expiry}
            onChange={setExpiry}
            disabled={mutation.isPending}
          />
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                mutation.error instanceof ApiError &&
                  mutation.error.code === 'api_key_limit'
                  ? 'keyLimitReached'
                  : mutation.error instanceof ApiError &&
                      mutation.error.code === 'group_unavailable'
                    ? 'groupAccessChanged'
                    : 'keyCreateFailed',
              )}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={close}
              disabled={mutation.isPending}
            >
              {t('cancel')}
            </Button>
            <Button
              type="submit"
              disabled={mutation.isPending || groups.isError || !selectedGroup}
            >
              {mutation.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t('createKey')}
            </Button>
          </DialogFooter>
        </form>
      )}
    </DialogContent>
  )
}
