// Types for palette.js, which src/jack/charts imports from TypeScript.
export declare const JACK_MUTE_CHROMA: number
export declare const JACK_MUTED_FAMILIES: string[]
export declare const JACK_NEUTRAL_FAMILIES: string[]
export declare function scaleChroma(rgb: [number, number, number], factor: number): [number, number, number]
export declare function jackMute(color: string, factor?: number): string
