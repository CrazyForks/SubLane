import { useRef, useState } from 'react'
import { MoreHorizontal, Gauge, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Member } from '@/lib/members'
import { Button } from './ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import { MemberLimits } from './MemberLimits'
import { PasswordDialog } from './PasswordDialog'

export function MemberActions({
  member,
  onPasswordReset,
}: {
  member: Member
  onPasswordReset?: () => void
}) {
  const { t } = useTranslation()
  const [action, setAction] = useState<'limits' | 'password' | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const focus = () => trigger.current?.focus()
  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button
            ref={trigger}
            variant="ghost"
            size="icon"
            aria-label={t('memberActionsFor', { username: member.username })}
          >
            <MoreHorizontal aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setAction('limits')}>
            <Gauge aria-hidden="true" />
            {t('memberLimits')}
          </DropdownMenuItem>
          {onPasswordReset && (
            <DropdownMenuItem onSelect={() => setAction('password')}>
              <KeyRound aria-hidden="true" />
              {t('resetPassword')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {action === 'limits' && (
        <MemberLimits
          member={member}
          onClose={() => setAction(null)}
          returnFocus={focus}
        />
      )}
      {action === 'password' && onPasswordReset && (
        <PasswordDialog
          member={member}
          onClose={() => setAction(null)}
          onSaved={onPasswordReset}
          returnFocus={focus}
        />
      )}
    </>
  )
}
