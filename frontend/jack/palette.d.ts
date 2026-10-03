// Types for palette.js, which src/jack/charts imports from TypeScript.
export type JackHue =
  | 'blueblack'
  | 'verdigris'
  | 'brass'
  | 'madder'
  | 'inkgrey'
  | 'mist'
  | 'slate'
  | 'lichen'
  | 'sienna'
  | 'rouge'
  | 'aubergine'
  | 'dai'
export declare const JACK_SHADES: number[]
export declare const JACK_LADDER: Array<[number, number]>
export declare const JACK_HUES: Record<JackHue, [number, number]>
export declare const JACK_UI_FAMILIES: Record<string, JackHue>
export declare const JACK_CHART_FAMILIES: Record<string, JackHue>
export declare const JACK_NEUTRAL_FAMILIES: string[]
export declare function oklchToHex(L: number, C: number, h: number): string
export declare function jackScale(hue: JackHue): Record<string, string>
export declare function jackFamilyColors(map: Record<string, string>): Record<string, Record<string, string>>
export declare function jackChartColor(color: string, grey?: (shade: string) => string | undefined): string
