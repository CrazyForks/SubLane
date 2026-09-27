import { useTranslation } from 'react-i18next'
import type { PriceCoverage } from '@/lib/allocations'

export function AllocationPriceWarning({
  coverage,
  label,
}: {
  coverage?: PriceCoverage
  label?: string
}) {
  const { t } = useTranslation()
  const missing = coverage?.missing_catalog_prices ?? []
  const uncovered = coverage?.uncovered_models ?? []
  if (!missing.length && !uncovered.length && !coverage?.catalog_unavailable)
    return null
  const names = (models: string[]) =>
    models.slice(0, 3).join(', ') + (models.length > 3 ? ', …' : '')
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
          })}
        </p>
      )}
      {coverage?.catalog_unavailable && (
        <p>{t('allocationCatalogUnavailable')}</p>
      )}
    </div>
  )
}
