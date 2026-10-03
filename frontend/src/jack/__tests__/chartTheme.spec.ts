// @vitest-environment node
import { describe, expect, it } from 'vitest'
import type { Chart } from 'chart.js'
import { createJackChartTheme } from '../charts/chartTheme'
import { jackChartColor } from '../../../jack/palette.js'

type Bag = Record<string, any>

const grey = (shade: string) => ({ '200': '232 232 229', '500': '119 121 126', '700': '62 64 69' })[shade]
const theme = createJackChartTheme(grey)

function fakeChart(datasets: Bag[], options: Bag = {}) {
  return { data: { datasets }, config: { options } } as unknown as Chart
}

function update(chart: Chart) {
  const beforeUpdate = theme.beforeUpdate as (chart: Chart, args: unknown, options: unknown) => void
  beforeUpdate(chart, {}, {})
}

describe('jackChartTheme', () => {
  it('maps dataset colours, strings and arrays alike', () => {
    const dataset = { borderColor: '#3b82f6', backgroundColor: ['#10b981', '#ef4444'], data: [1, 2] }
    update(fakeChart([dataset]))

    expect(dataset.borderColor).toBe(jackChartColor('#3b82f6'))
    expect(dataset.backgroundColor).toEqual([jackChartColor('#10b981'), jackChartColor('#ef4444')])
    expect(dataset.borderColor).not.toBe('#3b82f6')
  })

  it('does not map the same value twice across updates', () => {
    const dataset = { borderColor: '#3b82f6' }
    const chart = fakeChart([dataset])
    update(chart)
    const first = dataset.borderColor
    update(chart)

    expect(dataset.borderColor).toBe(first)
  })

  it('maps a colour that upstream replaces after an update', () => {
    const dataset: Bag = { borderColor: '#3b82f6' }
    const chart = fakeChart([dataset])
    update(chart)
    dataset.borderColor = '#ef4444'
    update(chart)

    expect(dataset.borderColor).toBe(jackChartColor('#ef4444'))
  })

  it('leaves scriptable colours alone', () => {
    const gradient = () => 'red'
    const dataset = { backgroundColor: gradient }
    update(fakeChart([dataset]))

    expect(dataset.backgroundColor).toBe(gradient)
  })

  it('maps axis and legend colours, greys through the theme tokens', () => {
    const options = {
      scales: { x: { grid: { color: '#e5e7eb' }, ticks: { color: '#6b7280' } }, y: { ticks: { color: '#3b82f6' } } },
      plugins: { legend: { labels: { color: '#374151' } } }
    }
    update(fakeChart([], options))

    expect(options.scales.x.grid.color).toBe('#e8e8e5')
    expect(options.scales.x.ticks.color).toBe('#77797e')
    expect(options.scales.y.ticks.color).toBe(jackChartColor('#3b82f6'))
    expect(options.plugins.legend.labels.color).toBe('#3e4045')
  })
})
