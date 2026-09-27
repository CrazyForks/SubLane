import { useTranslation } from 'react-i18next'
import type { PriceCoverage } from '@/lib/allocations'
import { formatInstanceDate, useTimeZone } from '@/lib/timezone'

export function AllocationPriceWarning({
  coverage,
  label,
  effectiveAt,
}: {
  coverage?: PriceCoverage
  label?: string
  effectiveAt?: number
}) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const missing = coverage?.missing_catalog_prices ?? []
  const uncovered = coverage?.uncovered_models ?? []
  if (!missing.length && !uncovered.length && !coverage?.catalog_unavailable)
    return null
  const names = (models: string[]) =>
    models.slice(0, 3).join(', ') + (models.length > 3 ? ', …' : '')
  const effectiveDate = effectiveAt
    ? formatInstanceDate(
        effectiveAt * 1000,
        i18n.resolvedLanguage ?? 'en',
        timeZone,
        { dateStyle: 'medium', timeStyle: 'short' },
      )
    : ''
  return (
    <div role="status" className="space-y-1 text-sm leading-6 text-warning">
      {label && <p className="font-medium">{label}</p>}
      {missing.length > 0 && (
        <p>
          {t('allocationMissingPrices', {
            count: missing.length,
            models: names(missing),
          })}
        </p>
      )}
      {uncovered.length > 0 && (
        <p>
          {t('allocationUncoveredModels', {
            count: uncovered.length,
            models: names(uncovered),
          })}{' '}
          {effectiveDate &&
            t('allocationUncoveredModelsTiming', { date: effectiveDate })}
        </p>
      )}
      {coverage?.catalog_unavailable && (
        <p>{t('allocationCatalogUnavailable')}</p>
      )}
    </div>
  )
}
