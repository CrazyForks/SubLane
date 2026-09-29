import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronsUpDown, LogOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authOptions, signOut } from '@/lib/auth'
import { replaceAuthState } from '@/lib/query'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from './ui/Sidebar'

export function Session() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const { isMobile } = useSidebar()
  const { data } = useQuery(authOptions())
  const username = data?.user?.username ?? ''
  const initials = username.slice(0, 2).toUpperCase()
  const logout = useMutation({
    meta: { demoAllowed: true },
    mutationFn: signOut,
    onSuccess: (state) => replaceAuthState(client, state),
  })
  const identity = (
    <>
      <span
        aria-hidden="true"
        className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent text-sm font-medium text-accent-foreground"
      >
        {initials}
      </span>
      <span className="grid min-w-0 flex-1 gap-0.5 text-left text-sm leading-tight">
        <span className="truncate font-semibold">{username}</span>
        <span className="truncate text-xs text-muted-foreground">
          {t(data?.user?.role === 'admin' ? 'administrator' : 'member')}
        </span>
      </span>
    </>
  )
  return (
    <>
      <SidebarMenu>
        <SidebarMenuItem>
          {/* The mobile sheet owns the modal lock; a second lock can outlive sign-out navigation. */}
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger asChild>
              <SidebarMenuButton
                size="lg"
                className="h-14 gap-3 rounded-lg"
                aria-label={t('accountMenu')}
                disabled={logout.isPending}
              >
                {identity}
                <ChevronsUpDown
                  aria-hidden="true"
                  className="ml-auto size-4 text-muted-foreground"
                />
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              side={isMobile ? 'top' : 'right'}
              align="end"
              sideOffset={8}
              className="w-60 max-w-[calc(100vw-2rem)] rounded-lg"
            >
              <DropdownMenuLabel className="flex items-center gap-3 p-2 font-normal">
                {identity}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-error focus:bg-error-muted focus:text-error [&_svg]:text-error"
                onSelect={() => logout.mutate()}
                disabled={logout.isPending}
              >
                <LogOut aria-hidden="true" />
                {t('signOut')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>
      {logout.isPending && (
        <p role="status" className="px-2 text-xs text-muted-foreground">
          {t('signingOut')}
        </p>
      )}
      {logout.isError && (
        <p role="alert" className="px-2 text-xs text-error">
          {t('signOutError')}
        </p>
      )}
    </>
  )
}
