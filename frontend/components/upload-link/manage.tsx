'use client'

import { Button } from '@/components/ui/button'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { formatDate } from '@/lib/i18n/core'
import { uploadLinkMessages as m } from '@/lib/i18n/messages/upload-link'
import { linkBase, useUploadLink } from '@/lib/upload-link'
import { CopyBlock } from '@/components/copy-block'

// Owner-only token management on the profile: create/rotate, revoke, last use.
export function UploadLinkManage() {
  const t = useT(m)
  const { status, token, error, rotate, revoke } = useUploadLink()
  return (
    <section>
      <SectionTitle>{t('title')}</SectionTitle>
      <div className="space-y-3 rounded-xl border border-border bg-card p-4 sm:p-5">
        <p className="max-w-2xl text-sm text-muted-foreground">{t('intro')}</p>
        {status && (
          <p className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={status.active ? 'default' : 'outline'}>{status.active ? t('active') : t('inactive')}</Badge>
            {status.active && status.created_at && (
              <span className="text-xs text-muted-foreground">
                {t('created', { date: formatDate(t.locale, status.created_at) })}, {status.last_used_at ? t('lastUsed', { date: formatDate(t.locale, status.last_used_at) }) : t('neverUsed')}
              </span>
            )}
          </p>
        )}
        {token && <div className="space-y-1.5"><p className="text-xs text-muted-foreground">{t('link')}</p><CopyBlock text={linkBase(token)} /></div>}
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={!status} onClick={() => { if (!status?.active || window.confirm(t('rotateConfirm'))) void rotate() }}>
            {status?.active ? t('rotate') : t('create')}
          </Button>
          {status?.active && <Button size="sm" variant="outline" onClick={() => { if (window.confirm(t('revokeConfirm'))) void revoke() }}>{t('revoke')}</Button>}
        </div>
        {error != null && <p role="alert" className="text-sm text-destructive">{t('failed', { error: errorText(error, t.locale) })}</p>}
      </div>
    </section>
  )
}
