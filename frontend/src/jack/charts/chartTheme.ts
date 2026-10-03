/**
 * Chart.js plugin that gives upstream charts the Jack mineral palette.
 *
 * Upstream charts set Tailwind's saturated colours in script (`#3b82f6`,
 * `rgba(16, 185, 129, 0.1)`, ...), which CSS cannot reach because Chart.js paints
 * on a canvas. Before each update this plugin passes dataset, axis and legend
 * colours through `jackMute`, the same function the Tailwind palette uses, so
 * charts match the badges and chips around them. Scriptable (function) colours
 * and gradients are left alone.
 */
import type { Plugin } from 'chart.js'
import { jackMute } from '../../../jack/palette.js'

type Bag = Record<string, unknown>

const DATASET_COLOR_KEYS = [
  'backgroundColor',
  'borderColor',
  'pointBackgroundColor',
  'pointBorderColor',
  'pointHoverBackgroundColor',
  'pointHoverBorderColor',
  'hoverBackgroundColor',
  'hoverBorderColor'
]

/** Values this plugin wrote, so an unchanged value is not muted a second time. */
const written = new WeakMap<object, Map<string, unknown>>()

function muteValue(value: unknown): unknown {
  if (typeof value === 'string') return jackMute(value)
  if (Array.isArray(value)) return value.map((item) => (typeof item === 'string' ? jackMute(item) : item))
  return value
}

function muteKey(target: unknown, key: string) {
  if (!target || typeof target !== 'object') return
  const bag = target as Bag
  const value = bag[key]
  if (value === undefined || value === null || typeof value === 'function') return
  const record = written.get(bag) ?? new Map<string, unknown>()
  if (record.has(key) && record.get(key) === value) return
  const muted = muteValue(value)
  bag[key] = muted
  record.set(key, muted)
  written.set(bag, record)
}

function child(target: unknown, key: string): unknown {
  return target && typeof target === 'object' ? (target as Bag)[key] : undefined
}

export const jackChartTheme: Plugin = {
  id: 'jackTheme',
  beforeUpdate(chart) {
    for (const dataset of chart.data.datasets) {
      for (const key of DATASET_COLOR_KEYS) muteKey(dataset, key)
    }

    // Raw config options: Chart.js resolves scales after this hook runs.
    const options = chart.config.options as Bag | undefined
    const scales = child(options, 'scales')
    if (scales && typeof scales === 'object') {
      for (const scale of Object.values(scales as Bag)) {
        for (const part of ['grid', 'ticks', 'border', 'title']) muteKey(child(scale, part), 'color')
      }
    }
    muteKey(child(child(child(options, 'plugins'), 'legend'), 'labels'), 'color')
  }
}
