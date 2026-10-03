// @vitest-environment node
import { describe, expect, it } from 'vitest'
import type { Chart } from 'chart.js'
import { jackChartTheme } from '../charts/chartTheme'
import { jackMute } from '../../../jack/palette.js'

type Bag = Record<string, any>

function fakeChart(datasets: Bag[], options: Bag = {}) {
  return { data: { datasets }, config: { options } } as unknown as Chart
}

function update(chart: Chart) {
  const beforeUpdate = jackChartTheme.beforeUpdate as (chart: Chart, args: unknown, options: unknown) => void
  beforeUpdate(chart, {}, {})
}

describe('jackChartTheme', () => {
  it('mutes dataset colours, strings and arrays alike', () => {
    const dataset = { borderColor: '#3b82f6', backgroundColor: ['#10b981', '#ef4444'], data: [1, 2] }
    update(fakeChart([dataset]))

    expect(dataset.borderColor).toBe(jackMute('#3b82f6'))
    expect(dataset.backgroundColor).toEqual([jackMute('#10b981'), jackMute('#ef4444')])
  })

  it('does not mute the same value twice across updates', () => {
    const dataset = { borderColor: '#3b82f6' }
    const chart = fakeChart([dataset])
    update(chart)
    update(chart)

    expect(dataset.borderColor).toBe(jackMute('#3b82f6'))
  })

  it('mutes a colour that upstream replaces after an update', () => {
    const dataset: Bag = { borderColor: '#3b82f6' }
    const chart = fakeChart([dataset])
    update(chart)
    dataset.borderColor = '#ef4444'
    update(chart)

    expect(dataset.borderColor).toBe(jackMute('#ef4444'))
  })

  it('leaves scriptable colours alone', () => {
    const gradient = () => 'red'
    const dataset = { backgroundColor: gradient }
    update(fakeChart([dataset]))

    expect(dataset.backgroundColor).toBe(gradient)
  })

  it('mutes axis and legend colours', () => {
    const options = {
      scales: { x: { grid: { color: '#e5e7eb' }, ticks: { color: '#6b7280' } }, y: { ticks: { color: '#3b82f6' } } },
      plugins: { legend: { labels: { color: '#374151' } } }
    }
    update(fakeChart([], options))

    expect(options.scales.x.grid.color).toBe(jackMute('#e5e7eb'))
    expect(options.scales.y.ticks.color).toBe(jackMute('#3b82f6'))
    expect(options.plugins.legend.labels.color).toBe(jackMute('#374151'))
  })
})
