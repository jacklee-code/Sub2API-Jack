/**
 * Jack wrapper around chart.js, swapped in by the jack-theme Vite plugin for every
 * upstream `import ... from 'chart.js'`. It re-exports chart.js unchanged and
 * registers the Jack chart theme once, before any upstream chart is created.
 * Its own `chart.js` import resolves to the real package.
 */
import { Chart } from 'chart.js'
import { jackChartTheme } from './chartTheme'

Chart.register(jackChartTheme)

export * from 'chart.js'
