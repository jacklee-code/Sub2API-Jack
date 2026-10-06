/** Natural width/height of an image attachment, falling back to an aspect ratio like "16:9". */
export function naturalSize(width: number | undefined, height: number | undefined, aspect?: string): { w: number; h: number } {
  if (width && height && width > 0 && height > 0) return { w: width, h: height }
  const [a, b] = String(aspect || '1:1').split(':').map(Number)
  if (a > 0 && b > 0) return { w: a, h: b }
  return { w: 1, h: 1 }
}

/** Display box that keeps the ratio and fits the longest edge into maxEdge. */
export function fitBox(width: number | undefined, height: number | undefined, maxEdge: number, aspect?: string): { width: number; height: number } {
  const { w, h } = naturalSize(width, height, aspect)
  const scale = maxEdge / Math.max(w, h)
  return { width: Math.round(w * scale), height: Math.round(h * scale) }
}
