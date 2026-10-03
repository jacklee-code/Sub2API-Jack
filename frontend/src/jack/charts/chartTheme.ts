/**
 * Chart.js plugin that gives upstream charts the Jack 文房 chart palette.
 *
 * Upstream charts set Tailwind colours in script (`#3b82f6`,
 * `rgba(16, 185, 129, 0.1)`, `#374151` for grid lines, ...), which CSS cannot
 * reach because Chart.js paints on a canvas. Before each update this plugin maps
 * every Tailwind default colour in datasets, axes and the legend to the Jack
 * colour for the same family and shade (palette.js JACK_CHART_FAMILIES), keeping
 * alpha. Greys follow the theme's `--jack-gray-*` tokens. Scriptable (function)
 * colours, gradients and colours that are not Tailwind defaults are left alone.
 */
import type { Plugin } from 'chart.js'
import { jackChartColor } from '../../../jack/palette.js'

type Bag = Record<string, unknown>
export type GreyResolver = (shade: string) => string | undefined

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

/** Reads a Jack grey token (`--jack-gray-500` → `119 121 126`) from the document. */
export const documentGrey: GreyResolver = (shade) => {
  if (typeof document === 'undefined') return undefined
  return getComputedStyle(document.documentElement).getPropertyValue(`--jack-gray-${shade}`) || undefined
}

export function createJackChartTheme(grey: GreyResolver = documentGrey): Plugin {
  /** Values this plugin wrote, so an unchanged value is not mapped again. */
  const written = new WeakMap<object, Map<string, unknown>>()

  const mapValue = (value: unknown): unknown => {
    if (typeof value === 'string') return jackChartColor(value, grey)
    if (Array.isArray(value)) return value.map((item) => (typeof item === 'string' ? jackChartColor(item, grey) : item))
    return value
  }

  const mapKey = (target: unknown, key: string) => {
    if (!target || typeof target !== 'object') return
    const bag = target as Bag
    const value = bag[key]
    if (value === undefined || value === null || typeof value === 'function') return
    const record = written.get(bag) ?? new Map<string, unknown>()
    if (record.has(key) && record.get(key) === value) return
    const mapped = mapValue(value)
    bag[key] = mapped
    record.set(key, mapped)
    written.set(bag, record)
  }

  const child = (target: unknown, key: string): unknown =>
    target && typeof target === 'object' ? (target as Bag)[key] : undefined

  return {
    id: 'jackTheme',
    beforeUpdate(chart) {
      for (const dataset of chart.data.datasets) {
        for (const key of DATASET_COLOR_KEYS) mapKey(dataset, key)
      }

      // Raw config options: Chart.js resolves scales after this hook runs.
      const options = chart.config.options as Bag | undefined
      const scales = child(options, 'scales')
      if (scales && typeof scales === 'object') {
        for (const scale of Object.values(scales as Bag)) {
          for (const part of ['grid', 'ticks', 'border', 'title']) mapKey(child(scale, part), 'color')
        }
      }
      mapKey(child(child(child(options, 'plugins'), 'legend'), 'labels'), 'color')
    }
  }
}

export const jackChartTheme = createJackChartTheme()
