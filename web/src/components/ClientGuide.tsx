import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { BookOpen, ChevronDown, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { connectionOptions } from '@/lib/connection'
import {
  clientConfiguration,
  clientProtocols,
  type ClientProtocol,
} from '@/lib/client'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Status } from './Status'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from './ui/Sheet'

export function ClientGuide({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [model, setModel] = useState('')
  const [protocol, setProtocol] = useState<ClientProtocol>('codex')
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="outline">
          <BookOpen aria-hidden="true" />
          {t('clientGuideAction')}
        </Button>
      </SheetTrigger>
      {open && (
        <SheetContent className="h-dvh w-full gap-0 sm:max-w-xl">
          <SheetHeader className="shrink-0 border-b border-border px-6 py-5 pe-14">
            <SheetTitle>{t('clientGuideTitle')}</SheetTitle>
            <SheetDescription className="leading-6">
              {t('clientGuideDescription')}
            </SheetDescription>
          </SheetHeader>
          <div className="min-h-0 min-w-0 flex-1 overflow-y-auto p-6">
            <GuideContent
              userID={userID}
              model={model}
              onModelChange={setModel}
              protocol={protocol}
              onProtocolChange={setProtocol}
            />
          </div>
        </SheetContent>
      )}
    </Sheet>
  )
}

function GuideContent({
  userID,
  model,
  onModelChange,
  protocol,
  onProtocolChange,
}: {
  userID: number
  model: string
  onModelChange: (value: string) => void
  protocol: ClientProtocol
  onProtocolChange: (value: ClientProtocol) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(connectionOptions(client, userID))
  const [copiedConfiguration, setCopiedConfiguration] = useState<string | null>(
    null,
  )
  const [copyFailed, setCopyFailed] = useState(false)
  const { baseURL: endpoint, configuration } = clientConfiguration(
    protocol,
    window.location.origin,
    model,
  )
  const copied = copiedConfiguration === configuration
  const protocolLabels = {
    codex: 'clientProtocolCodex',
    openai: 'clientProtocolOpenAI',
    claude: 'clientProtocolClaude',
    gemini: 'clientProtocolGemini',
  } as const
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(configuration)
      setCopiedConfiguration(configuration)
      setCopyFailed(false)
    } catch {
      setCopyFailed(true)
      setCopiedConfiguration(null)
    }
  }
  return (
    <div className="min-w-0 space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Status
          kind={
            query.isError || query.isPending
              ? 'neutral'
              : query.data?.status === 'ready'
                ? 'success'
                : 'warning'
          }
        >
          {t(
            query.isPending
              ? 'gatewayChecking'
              : query.isError
                ? 'gatewayStatusUnknown'
                : query.data?.status === 'ready'
                  ? 'ready'
                  : query.data?.status === 'needs_attention'
                    ? 'gatewayNeedsAttention'
                    : 'notConfigured',
          )}
        </Status>
      </div>
      {query.isError ? (
        <p className="text-sm text-muted-foreground">
          {t('connectionStatusError')}
        </p>
      ) : query.data && query.data.status !== 'ready' ? (
        <p className="text-sm text-muted-foreground">
          {t('connectionNotConfigured')}
        </p>
      ) : null}
      <div className="space-y-2">
        <label htmlFor="client-protocol" className="text-sm font-medium">
          {t('clientProtocol')}
        </label>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              id="client-protocol"
              variant="outline"
              aria-label={t('clientProtocol')}
              className="w-full justify-between font-normal"
            >
              {t(protocolLabels[protocol])}
              <ChevronDown
                aria-hidden="true"
                className="text-muted-foreground"
              />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            className="w-(--radix-dropdown-menu-trigger-width)"
          >
            <DropdownMenuRadioGroup
              value={protocol}
              onValueChange={(value) => {
                const selected = clientProtocols.find((item) => item === value)
                if (selected) {
                  onProtocolChange(selected)
                  setCopyFailed(false)
                }
              }}
            >
              {clientProtocols.map((item) => (
                <DropdownMenuRadioItem key={item} value={item}>
                  {t(protocolLabels[item])}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {protocol === 'openai' && (
        <p className="text-sm leading-6 text-muted-foreground">
          {t('clientClineInstructions')}
        </p>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label htmlFor="client-endpoint" className="text-sm font-medium">
            {t('clientEndpoint')}
          </label>
          <Input
            id="client-endpoint"
            value={endpoint}
            readOnly
            className="font-mono text-sm"
          />
        </div>
        <div className="space-y-2">
          <label htmlFor="client-model" className="text-sm font-medium">
            {t('clientModel')}
          </label>
          <Input
            id="client-model"
            value={model}
            onChange={(event) => {
              onModelChange(event.target.value)
              setCopyFailed(false)
            }}
            maxLength={128}
            aria-describedby="client-model-hint"
          />
          <p id="client-model-hint" className="text-xs text-muted-foreground">
            {t('clientModelHint')}
          </p>
        </div>
      </div>
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-medium">
            {t(protocol === 'codex' ? 'clientConfig' : 'clientRequestExample')}
          </h3>
          <Button variant="ghost" size="sm" onClick={copy}>
            <Copy aria-hidden="true" />
            {t(copied ? 'copied' : 'copyClientConfig')}
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          {t(
            protocol === 'codex'
              ? 'clientConfigLocation'
              : 'clientRequestInstructions',
          )}
        </p>
        <pre className="overflow-x-auto rounded-lg bg-muted p-4 text-xs leading-6">
          <code>{configuration}</code>
        </pre>
      </div>
      {copyFailed && (
        <p role="alert" className="text-sm text-error">
          {t('copyKeyFailed')}
        </p>
      )}
      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
        {t(protocol === 'codex' ? 'clientGuideNote' : 'clientNativeGuideNote')}
      </p>
    </div>
  )
}
