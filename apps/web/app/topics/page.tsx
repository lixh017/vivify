'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api, type GeneratedTopic } from '@/lib/api'
import type { Topic } from '@/lib/types'

const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']
const STATUSES = ['idea', 'writing', 'review', 'approved', 'archived']

interface FormState {
  title: string
  angle: string
  platform: string
  status: string
}

const EMPTY_FORM: FormState = {
  title: '',
  angle: '',
  platform: PLATFORMS[0],
  status: STATUSES[0],
}

export default function TopicsPage() {
  const [topics, setTopics] = useState<Topic[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [filterPlatform, setFilterPlatform] = useState<string>('')
  const [filterStatus, setFilterStatus] = useState<string>('')

  // AI topic generation state. aiModalOpen drives a small inline
  // panel for seed/count; aiResult holds the latest batch; aiError
  // surfaces server failures (e.g. 503 when no API key configured).
  const [aiModalOpen, setAiModalOpen] = useState(false)
  const [aiSeed, setAiSeed] = useState('')
  const [aiCount, setAiCount] = useState(5)
  const [aiLoading, setAiLoading] = useState(false)
  const [aiResult, setAiResult] = useState<GeneratedTopic[] | null>(null)
  const [aiError, setAiError] = useState<string | null>(null)

  async function loadTopics() {
    setLoading(true)
    setError(null)
    try {
      const params: { platform?: string; status?: string } = {}
      if (filterPlatform) params.platform = filterPlatform
      if (filterStatus) params.status = filterStatus
      const res = await api.topics.list(params)
      setTopics(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadTopics()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterPlatform, filterStatus])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.topics.create({
        title: form.title,
        angle: form.angle,
        platform: form.platform,
        status: form.status,
      })
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadTopics()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  // handleGenerateTopics asks Claude for N differentiated topic
  // ideas and stores the result for rendering. aiModalOpen is left
  // open so the user can tweak the seed and try again.
  async function handleGenerateTopics() {
    if (!aiSeed.trim()) {
      setAiError('请输入种子概念')
      return
    }
    setAiLoading(true)
    setAiError(null)
    setAiResult(null)
    try {
      const res = await api.ai.generateTopics({
        seed: aiSeed.trim(),
        platform: filterPlatform || PLATFORMS[0],
        count: aiCount,
      })
      setAiResult(res.topics)
    } catch (err: unknown) {
      setAiError(err instanceof Error ? err.message : 'AI 生成失败')
    } finally {
      setAiLoading(false)
    }
  }

  // applyGeneratedTopic pre-fills the create form with a generated
  // topic and closes the AI panel. Keeps the user in flow: click an
  // idea -> tweak the angle -> save.
  function applyGeneratedTopic(t: GeneratedTopic) {
    setForm({
      title: t.title,
      angle: `${t.angle}\n\n预期表现：${t.expected_performance}\n钩子：${t.hook}`,
      platform: filterPlatform || PLATFORMS[0],
      status: STATUSES[0],
    })
    setShowForm(true)
    setAiResult(null)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📋 选题</h1>
        <div className="flex gap-2">
          <button
            onClick={() => {
              setAiModalOpen((v) => !v)
              setAiError(null)
            }}
            className="px-4 py-2 bg-purple-600 text-white rounded-lg shadow hover:bg-purple-700"
            disabled={aiLoading}
          >
            {aiLoading ? '生成中...' : 'AI 生成选题'}
          </button>
          <button
            onClick={() => setShowForm((v) => !v)}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
          >
            {showForm ? '取消' : '+ 新建选题'}
          </button>
        </div>
      </div>

      <div className="flex gap-3 items-center">
        <label className="text-sm text-gray-600">
          平台:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="ml-2 px-2 py-1 border border-gray-300 rounded"
          >
            <option value="">全部</option>
            {PLATFORMS.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="text-sm text-gray-600">
          状态:
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="ml-2 px-2 py-1 border border-gray-300 rounded"
          >
            <option value="">全部</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
      </div>

      {aiModalOpen && (
        <div className="p-4 bg-purple-50 border border-purple-200 rounded-lg space-y-3">
          <h2 className="font-semibold text-purple-900">AI 选题生成</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div className="md:col-span-2">
              <label className="block text-sm font-medium text-gray-700">
                种子概念
              </label>
              <input
                type="text"
                value={aiSeed}
                onChange={(e) => setAiSeed(e.target.value)}
                placeholder="例如：禅意解压、深夜emo、宅文化..."
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">
                数量
              </label>
              <input
                type="number"
                min={1}
                max={20}
                value={aiCount}
                onChange={(e) => setAiCount(parseInt(e.target.value, 10) || 1)}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              />
            </div>
          </div>
          <div className="text-sm text-gray-600">
            平台：<span className="font-medium">{filterPlatform || PLATFORMS[0]}</span>
            {filterPlatform ? '' : '（默认；可用上方筛选器切换）'}
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleGenerateTopics}
              disabled={aiLoading}
              className="px-4 py-2 bg-purple-600 text-white rounded shadow hover:bg-purple-700 disabled:opacity-50"
            >
              {aiLoading ? '生成中...' : '生成'}
            </button>
            <button
              onClick={() => {
                setAiModalOpen(false)
                setAiError(null)
                setAiResult(null)
              }}
              className="px-4 py-2 bg-white text-gray-700 rounded shadow border border-gray-300 hover:bg-gray-50"
            >
              关闭
            </button>
          </div>
          {aiError && (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
              {aiError}
            </div>
          )}
          {aiResult && aiResult.length > 0 && (
            <div className="space-y-2">
              <div className="text-sm text-gray-600">
                点击任意选题可填入下方创建表单：
              </div>
              {aiResult.map((t, i) => (
                <button
                  key={`${t.title}-${i}`}
                  onClick={() => applyGeneratedTopic(t)}
                  className="w-full text-left p-3 bg-white border border-purple-200 rounded shadow-sm hover:border-purple-500 hover:shadow"
                >
                  <div className="font-semibold text-gray-900">{t.title}</div>
                  <div className="text-sm text-gray-700 mt-1">{t.angle}</div>
                  <div className="text-xs text-gray-500 mt-2">
                    <span className="font-medium">预期：</span>
                    {t.expected_performance}
                  </div>
                  <div className="text-xs text-gray-500">
                    <span className="font-medium">钩子：</span>
                    {t.hook}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label className="block text-sm font-medium text-gray-700">
              标题
            </label>
            <input
              type="text"
              required
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              角度
            </label>
            <textarea
              required
              value={form.angle}
              onChange={(e) => setForm({ ...form, angle: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              rows={3}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700">
                平台
              </label>
              <select
                value={form.platform}
                onChange={(e) => setForm({ ...form, platform: e.target.value })}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              >
                {PLATFORMS.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">
                状态
              </label>
              <select
                value={form.status}
                onChange={(e) => setForm({ ...form, status: e.target.value })}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              >
                {STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700 disabled:opacity-50"
          >
            {submitting ? '提交中...' : '保存'}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-gray-600">Loading...</div>
      ) : topics.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-3">
          {topics.map((t) => (
            <div
              key={t.id}
              className="p-4 bg-white rounded-lg shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <h2 className="font-semibold text-lg">{t.title}</h2>
                  <p className="text-sm text-gray-600 mt-1">{t.angle}</p>
                </div>
                <div className="flex gap-2 text-xs">
                  <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded">
                    {t.platform}
                  </span>
                  <span className="px-2 py-1 bg-gray-100 text-gray-700 rounded">
                    {t.status}
                  </span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
