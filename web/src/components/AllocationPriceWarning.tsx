import { useTranslation } from 'react-i18next'
import type { AllocationConfig } from '@/lib/allocations'

export function AllocationPriceWarning({
  config,
  label,
}: {
  config: AllocationConfig
  label?: string
}) {
  const { t } = useTranslation()
  const missing = config.missing_price_models ?? []
  if (!missing.length && !config.catalog_unavailable) return null
  const models =
    missing.slice(0, 3).join(', ') + (missing.length > 3 ? ', …' : '')
  return (
    <div role="status" className="space-y-1 text-sm leading-6 text-warning">
      {label && <p className="font-medium">{label}</p>}
      {missing.length > 0 && (
        <p>{t('allocationMissingPrices', { count: missing.length, models })}</p>
      )}
      {config.catalog_unavailable && <p>{t('allocationCatalogUnavailable')}</p>}
    </div>
  )
}
