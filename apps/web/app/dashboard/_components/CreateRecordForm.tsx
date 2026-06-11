'use client'

import { FormEvent } from 'react'
import { useT } from '@opc/shared/i18n-client'

// Canonical platform vocabulary mirroring the backend's allowedPlatforms
// set. Wire values; the surrounding chrome is rendered via i18n.
export const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']

export interface FormState {
  platform: string
  platform_url: string
  performance_metrics: string
}

export const EMPTY_FORM: FormState = {
  platform: PLATFORMS[0],
  platform_url: '',
  performance_metrics: '',
}

interface CreateRecordFormProps {
  form: FormState
  submitting: boolean
  onChange: (next: FormState) => void
  onSubmit: (e: FormEvent<HTMLFormElement>) => void
}

export function CreateRecordForm({ form, submitting, onChange, onSubmit }: CreateRecordFormProps) {
  const t = useT()
  return (
    <form
      onSubmit={onSubmit}
      className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
    >
      <div>
        <label className="block text-xs md:text-sm font-medium text-claude-ink">
          {t('dashboard.form.platform_label')}
        </label>
        <select
          data-testid="input-test-platform"
          value={form.platform}
          onChange={(e) => onChange({ ...form, platform: e.target.value })}
          className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
        >
          {PLATFORMS.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
      </div>
      <div>
        <label className="block text-xs md:text-sm font-medium text-claude-ink">
          {t('dashboard.form.url_label')}
        </label>
        <input
          type="url"
          data-testid="input-test-platform_url"
          value={form.platform_url}
          onChange={(e) =>
            onChange({ ...form, platform_url: e.target.value })
          }
          placeholder="https://..."
          className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
        />
      </div>
      <div>
        <label className="block text-xs md:text-sm font-medium text-claude-ink">
          {t('dashboard.form.metrics_label')}
        </label>
        <textarea
          data-testid="input-test-performance_metrics"
          value={form.performance_metrics}
          onChange={(e) =>
            onChange({ ...form, performance_metrics: e.target.value })
          }
          placeholder={t('dashboard.form.metrics_placeholder')}
          className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
          rows={3}
        />
      </div>
      <button
        type="submit"
        data-testid="btn-submit"
        disabled={submitting}
        className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
      >
        {submitting
          ? t('dashboard.form.submit_loading')
          : t('dashboard.form.submit_idle')}
      </button>
    </form>
  )
}
