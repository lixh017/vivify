// Skill registry — single source of truth for every skill the OPC
// backend exposes. /docs (markdown-driven) and /sandbox (form
// generator) both import from this file so the two surfaces can
// never drift on names / params / paths.
//
// Adding a new skill: append an entry to SKILLS, the docs site
// regenerates at build time and the sandbox's dropdown picks it
// up automatically. No code changes to the consumer pages.
//
// Param schema is a tiny declarative shape — just enough for the
// sandbox to render a form (text / textarea / number / select /
// boolean). Full JSON Schema is overkill for the MCN operator
// audience.

export type ParamKind = 'text' | 'textarea' | 'number' | 'select' | 'boolean'

export interface ParamSchema {
  name: string
  label: string
  kind: ParamKind
  required: boolean
  default?: string | number | boolean
  // For kind=select: a list of {value,label} pairs.
  options?: { value: string; label: string }[]
  help?: string
  // When set, this param is omitted from the sandbox form (it's
  // a server-side param derived from the session, not the form).
  // The docs page still shows it in the schema reference.
  serverOnly?: boolean
}

export interface SkillMeta {
  id: string
  name: string
  category: 'ai' | 'media' | 'admin'
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: string
  description: string
  // Approximate cost in CNY cents. Documented; the actual cost
  // is computed server-side from provider pricing.
  costCentsHint: string
  // Free-form rate-limit note rendered on the docs page.
  rateLimit: string
  params: ParamSchema[]
  // A short example payload shown in the docs page. The sandbox
  // form can pre-fill from this for the "Run with example" button.
  example?: Record<string, unknown>
}

// All 8 AI skills the pipeline already exposes, plus the 2
// admin / media skills that round out the operator's mental
// model. Keep this list sorted by id so a future diff is easy
// to review.
export const SKILLS: SkillMeta[] = [
  {
    id: 'ai_topics',
    name: 'AI 选题',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/topics',
    description: '基于种子词生成 N 个差异化选题 (hook + 角度 + 预期表现)。',
    costCentsHint: '~0.4 分 / 次 (Haiku 4.5)',
    rateLimit: '120/min',
    params: [
      {
        name: 'seed',
        label: '种子词',
        kind: 'text',
        required: true,
        help: '一句或一个短语, 如 "雨夜"',
      },
      {
        name: 'platform',
        label: '目标平台',
        kind: 'select',
        required: true,
        default: '抖音',
        options: [
          { value: '抖音', label: '抖音' },
          { value: '哔哩哔哩', label: '哔哩哔哩' },
          { value: '小红书', label: '小红书' },
        ],
      },
      {
        name: 'count',
        label: '生成数量',
        kind: 'number',
        required: true,
        default: 3,
        help: '1-5 之间',
      },
    ],
    example: { seed: '雨夜', platform: '抖音', count: 3 },
  },
  {
    id: 'ai_humanize',
    name: 'AI 人化',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/humanize',
    description: '把 AI 味的脚本改写得更像人写, 去除模板化短语。',
    costCentsHint: '~0.8 分 / 次',
    rateLimit: '120/min',
    params: [
      {
        name: 'script',
        label: '原始脚本',
        kind: 'textarea',
        required: true,
        help: '通常 200-500 字',
      },
    ],
    example: { script: '今天想给大家讲讲庄子。' },
  },
  {
    id: 'ai_score',
    name: '内容评分',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/score',
    description: '对脚本按 hook / 结构 / 平台适配三个维度评分。',
    costCentsHint: '~0.6 分 / 次',
    rateLimit: '120/min',
    params: [
      { name: 'title', label: '标题', kind: 'text', required: true },
      { name: 'script', label: '脚本', kind: 'textarea', required: true },
      {
        name: 'platform',
        label: '目标平台',
        kind: 'select',
        required: true,
        default: '抖音',
        options: [
          { value: '抖音', label: '抖音' },
          { value: '哔哩哔哩', label: '哔哩哔哩' },
          { value: '小红书', label: '小红书' },
        ],
      },
    ],
  },
  {
    id: 'ai_platform_adapt',
    name: '平台适配',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/platform-adapt',
    description: '把同一个内容点子适配到三个平台 + 跨平台提示。',
    costCentsHint: '~0.7 分 / 次',
    rateLimit: '60/min',
    params: [
      { name: 'title', label: '标题', kind: 'text', required: true },
      { name: 'angle', label: '角度', kind: 'text', required: true },
      {
        name: 'source_platform',
        label: '源平台',
        kind: 'select',
        required: true,
        default: '抖音',
        options: [
          { value: '抖音', label: '抖音' },
          { value: '哔哩哔哩', label: '哔哩哔哩' },
          { value: '小红书', label: '小红书' },
        ],
      },
    ],
  },
  {
    id: 'ai_deconstruct',
    name: '爆款拆解',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/deconstruct',
    description: '对爆款视频文案做钩子 / 结构 / CTA / 情绪曲线拆解。',
    costCentsHint: '~1.2 分 / 次',
    rateLimit: '30/min',
    params: [
      {
        name: 'transcript',
        label: '视频文案 / 转写',
        kind: 'textarea',
        required: true,
      },
      {
        name: 'platform',
        label: '平台',
        kind: 'text',
        required: false,
        help: '可选, 抖音/小红书/哔哩哔哩',
      },
    ],
  },
  {
    id: 'ai_viral_formula',
    name: '爆款公式',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/viral-formula',
    description: '从拆解后的爆款里提取可复用公式 (变量 + 步骤 + 变体)。',
    costCentsHint: '~0.9 分 / 次',
    rateLimit: '30/min',
    params: [
      {
        name: 'transcript',
        label: '视频文案',
        kind: 'textarea',
        required: true,
      },
    ],
  },
  {
    id: 'ai_pipeline',
    name: '全链路 Pipeline',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/pipeline',
    description: '一次调用跑 选题 → 脚本 → 评分 → 平台适配, 1 个 API = 4 步。',
    costCentsHint: '~2.5 分 / 次',
    rateLimit: '20/min',
    params: [
      { name: 'seed', label: '种子词', kind: 'text', required: true },
      {
        name: 'platform',
        label: '目标平台',
        kind: 'select',
        required: true,
        default: '抖音',
        options: [
          { value: '抖音', label: '抖音' },
          { value: '哔哩哔哩', label: '哔哩哔哩' },
          { value: '小红书', label: '小红书' },
        ],
      },
      {
        name: 'include_knowledge',
        label: '注入知识库',
        kind: 'boolean',
        required: false,
        default: false,
      },
    ],
  },
  {
    id: 'ai_batch',
    name: '批量生成',
    category: 'ai',
    method: 'POST',
    path: '/api/ai/batch',
    description: '1 个 API 调用生成 N 个内容 (选题 / 评分 / 平台适配)。',
    costCentsHint: '~N × 单次',
    rateLimit: '10/min',
    params: [
      {
        name: 'task',
        label: '任务类型',
        kind: 'select',
        required: true,
        default: 'topics',
        options: [
          { value: 'topics', label: '选题' },
          { value: 'score', label: '评分' },
          { value: 'platform_adapt', label: '平台适配' },
        ],
      },
      {
        name: 'count',
        label: '数量 N',
        kind: 'number',
        required: true,
        default: 5,
      },
    ],
  },
  {
    id: 'cover_generate',
    name: '封面生成',
    category: 'media',
    method: 'POST',
    path: '/api/cover',
    description: '基于标题 + 风格 + 平台生成 9:16 (抖音/小红书) 或 16:9 (哔哩哔哩) 封面图。',
    costCentsHint: '~5 分 / 张 (Kling / 即梦)',
    rateLimit: '30/min',
    params: [
      { name: 'title', label: '标题', kind: 'text', required: true },
      {
        name: 'style',
        label: '风格',
        kind: 'text',
        required: false,
        default: '治愈系',
        help: '默认 "治愈系", 可填 "赛博朋克" / "手绘" 等',
      },
      {
        name: 'platform',
        label: '平台',
        kind: 'select',
        required: false,
        default: '抖音',
        options: [
          { value: '抖音', label: '抖音' },
          { value: '小红书', label: '小红书' },
          { value: '哔哩哔哩', label: '哔哩哔哩' },
        ],
      },
    ],
  },
  {
    id: 'tts_synthesize',
    name: 'TTS 配音',
    category: 'media',
    method: 'POST',
    path: '/api/speech',
    description: '把一段文本合成 mp3 配音。返回的 path 即可在浏览器中播放 (后续 #85 暴露静态文件)。',
    costCentsHint: '~1 分 / 1k 字 (minimax TTS 2.8 HD)',
    rateLimit: '60/min',
    params: [
      {
        name: 'text',
        label: '配音文本',
        kind: 'textarea',
        required: true,
        help: '要合成的文本内容',
      },
      {
        name: 'voice',
        label: '声音',
        kind: 'select',
        required: false,
        default: 'English_expressive_narrator',
        options: [
          { value: 'English_expressive_narrator', label: '英文旁白' },
          { value: 'Chinese_mandarin_calm', label: '中文平静' },
          { value: 'Chinese_mandarin_warm', label: '中文温暖' },
          { value: 'Chinese_mandarin_news', label: '中文新闻' },
        ],
      },
      {
        name: 'speed',
        label: '语速',
        kind: 'number',
        required: false,
        default: 1.0,
        help: '0.5 - 2.0',
      },
      {
        name: 'volume',
        label: '音量',
        kind: 'number',
        required: false,
        default: 1.0,
        help: '0 - 1.5',
      },
      {
        name: 'pitch',
        label: '音调',
        kind: 'number',
        required: false,
        default: 0,
        help: '-12 ~ 12 半音',
      },
      {
        name: 'language',
        label: '语言',
        kind: 'text',
        required: false,
        help: '可选, 例如 zh / en / ja',
      },
    ],
    example: {
      text: '今天的雨,下得有点久。',
      voice: 'Chinese_mandarin_warm',
      speed: 1.0,
    },
  },
  {
    id: 'render_video',
    name: '视频合成',
    category: 'media',
    method: 'POST',
    path: '/api/video',
    description: '基于 prompt 生成 5s 视频片段。支持首帧 / 末帧 / 主体参考 (I2V / SEF / S2V)。',
    costCentsHint: '~100 分 / 段 (minimax Hailuo 2.3)',
    rateLimit: '20/min',
    params: [
      {
        name: 'prompt',
        label: '提示词',
        kind: 'textarea',
        required: true,
        help: '描述想要的画面',
      },
      {
        name: 'first_frame',
        label: '首帧图',
        kind: 'text',
        required: false,
        help: '可选, 图像 URL 或本地路径 (I2V 模式)',
      },
      {
        name: 'last_frame',
        label: '末帧图',
        kind: 'text',
        required: false,
        help: '可选, 需配合 first_frame (SEF 模式)',
      },
      {
        name: 'subject_image',
        label: '主体参考图',
        kind: 'text',
        required: false,
        help: '可选, S2V 模式: 主体一致性参考',
      },
    ],
    example: {
      prompt: 'a panda eating bamboo in a misty bamboo forest, cinematic',
      first_frame: '',
    },
  },
  {
    id: 'credentials_list',
    name: '凭据列表 (admin)',
    category: 'admin',
    method: 'GET',
    path: '/api/credentials',
    description: '列出当前 operator 的所有 API 凭据 (不含密文)。',
    costCentsHint: '0 (内部)',
    rateLimit: 'n/a',
    params: [],
  },
]

// LOOKUP is a derived index for O(1) by-id access. Computed once
// at module load — the SKILLS array is small and static.
const LOOKUP: Record<string, SkillMeta> = Object.fromEntries(
  SKILLS.map((s) => [s.id, s]),
)

export function getSkill(id: string): SkillMeta | undefined {
  return LOOKUP[id]
}

export function skillsByCategory(
  category: SkillMeta['category'],
): SkillMeta[] {
  return SKILLS.filter((s) => s.category === category)
}
