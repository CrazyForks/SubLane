import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from './ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import type { ExpiryChoice } from '@/lib/keys'
import { formatInstanceDate, useTimeZone } from '@/lib/timezone'
export function KeyExpiry({
  value,
  onChange,
  current,
  disabled,
}: {
  value: ExpiryChoice
  onChange: (value: ExpiryChoice) => void
  current?: number | null
  disabled: boolean
}) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const label = (choice: ExpiryChoice) =>
    choice === 'keep'
      ? t('keyKeepExpiry')
      : choice === 'never'
        ? t('keyNeverExpires')
        : t('keyExpiryDays', { count: Number(choice) })
  const options: ExpiryChoice[] =
    current === undefined
      ? ['never', '7', '30', '90']
      : ['keep', 'never', '7', '30', '90']
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('keyExpiry')}</p>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className="w-full justify-between"
            aria-label={t('keyExpiry')}
            disabled={disabled}
          >
            {label(value)}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-(--radix-dropdown-menu-trigger-width)">
          <DropdownMenuRadioGroup
            value={value}
            onValueChange={(choice) => onChange(choice as ExpiryChoice)}
          >
            {options.map((choice) => (
              <DropdownMenuRadioItem key={choice} value={choice}>
                {label(choice)}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {value === 'keep' && (
        <p className="text-xs text-muted-foreground">
          {current === null
            ? t('keyNeverExpires')
            : t('keyExpiresOn', {
                date: formatInstanceDate(
                  current! * 1000,
                  i18n.resolvedLanguage ?? 'en',
                  timeZone,
                  { dateStyle: 'short', timeStyle: 'medium' },
                ),
              })}
        </p>
      )}
    </div>
  )
}
