<template>
  <canvas ref="canvas" class="jack-ink-canvas" aria-hidden="true"></canvas>
</template>

<script setup lang="ts">
/**
 * 墨暈: ink that blooms on paper where the pointer moves.
 *
 * Fills its positioned parent and listens to pointer movement on that parent.
 * Each drop is a pre-rendered blot sprite (irregular, feathered edge) that
 * spreads and fades. Light mode draws ink with `multiply`; dark mode draws a
 * pale mist with `screen`. A faint drop also falls now and then, so touch
 * screens see it too.
 *
 * Cost control: the animation loop only runs while drops are alive, stops when
 * the hero is off screen or the tab is hidden, and the backing store is capped
 * at 2x device pixels. With `prefers-reduced-motion` it paints two still blots
 * once. Without a 2D context (old browsers, tests) it renders nothing.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

interface Drop {
  x: number
  y: number
  from: number
  to: number
  age: number
  life: number
  alpha: number
  sprite: number
  angle: number
}

const SPRITE_SIZE = 256
const SPRITE_COUNT = 6
const MAX_DROPS = 70

const canvas = ref<HTMLCanvasElement | null>(null)

let ctx: CanvasRenderingContext2D | null = null
let host: HTMLElement | null = null
let sprites: HTMLCanvasElement[] = []
let drops: Drop[] = []
let frame = 0
let lastTime = 0
let lastPoint: { x: number; y: number } | null = null
let ambientTimer = 0
let visible = true
let dark = false
let width = 0
let height = 0
let resizeObserver: ResizeObserver | null = null
let intersectionObserver: IntersectionObserver | null = null
let themeObserver: MutationObserver | null = null
let reducedMotion = false

function random(min: number, max: number) {
  return min + Math.random() * (max - min)
}

/** A closed outline whose radius wobbles with a few random harmonics. */
function blotPath(g: CanvasRenderingContext2D, cx: number, cy: number, radius: number, roughness: number) {
  const harmonics = Array.from({ length: 5 }, (_, i) => ({
    freq: 2 + i * 2 + Math.floor(Math.random() * 2),
    amp: (random(0.4, 1) * roughness) / (i + 1),
    phase: random(0, Math.PI * 2)
  }))
  g.beginPath()
  for (let step = 0; step <= 120; step++) {
    const theta = (step / 120) * Math.PI * 2
    const wobble = harmonics.reduce((sum, h) => sum + h.amp * Math.sin(h.freq * theta + h.phase), 0)
    const r = radius * (1 + wobble)
    const x = cx + Math.cos(theta) * r
    const y = cy + Math.sin(theta) * r
    if (step === 0) g.moveTo(x, y)
    else g.lineTo(x, y)
  }
  g.closePath()
}

/**
 * One ink blot as it dries on xuan paper: a pale wash, a mottled body, the
 * darker tide line where pigment gathers at the edge, a few satellite specks
 * and fine granulation.
 */
function makeSprite(color: string): HTMLCanvasElement {
  const sprite = document.createElement('canvas')
  sprite.width = sprite.height = SPRITE_SIZE
  const g = sprite.getContext('2d')
  if (!g) return sprite
  const c = SPRITE_SIZE / 2
  const radius = SPRITE_SIZE * 0.3
  const ink = (alpha: number) => color.replace('ALPHA', alpha.toFixed(3))
  const blur = (px: number) => {
    if ('filter' in g) g.filter = px ? `blur(${px}px)` : 'none'
  }

  // Wash: the water front, pale and soft.
  blur(9)
  g.fillStyle = ink(0.22)
  blotPath(g, c, c, radius * 1.08, 0.16)
  g.fill()

  // Body: two offset lobes, so the density is uneven.
  blur(4)
  for (let i = 0; i < 2; i++) {
    g.fillStyle = ink(0.32)
    blotPath(g, c + random(-10, 10), c + random(-10, 10), radius * random(0.62, 0.8), 0.22)
    g.fill()
  }

  // Tide line: pigment collects where the water stops.
  blur(1.6)
  g.strokeStyle = ink(0.55)
  g.lineWidth = 2.2
  blotPath(g, c, c, radius * 0.98, 0.14)
  g.stroke()

  // Satellite specks thrown off the drop.
  blur(0.8)
  g.fillStyle = ink(0.6)
  for (let i = 0; i < 7; i++) {
    const theta = random(0, Math.PI * 2)
    const r = radius * random(1.1, 1.45)
    g.beginPath()
    g.arc(c + Math.cos(theta) * r, c + Math.sin(theta) * r, random(0.8, 3.2), 0, Math.PI * 2)
    g.fill()
  }

  // Granulation: pigment settling into the paper fibres.
  blur(0)
  g.globalCompositeOperation = 'source-atop'
  for (let i = 0; i < 260; i++) {
    const theta = random(0, Math.PI * 2)
    const r = radius * Math.sqrt(Math.random())
    g.fillStyle = ink(random(0.05, 0.22))
    g.fillRect(c + Math.cos(theta) * r, c + Math.sin(theta) * r, random(0.6, 1.8), random(0.6, 1.8))
  }
  g.globalCompositeOperation = 'source-over'
  return sprite
}

function buildSprites() {
  dark = document.documentElement.classList.contains('dark')
  const color = dark ? 'rgba(236, 236, 234, ALPHA)' : 'rgba(23, 24, 27, ALPHA)'
  sprites = Array.from({ length: SPRITE_COUNT }, () => makeSprite(color))
  if (canvas.value) canvas.value.style.mixBlendMode = dark ? 'screen' : 'multiply'
}

function resize() {
  if (!canvas.value || !host || !ctx) return
  const rect = host.getBoundingClientRect()
  const ratio = Math.min(window.devicePixelRatio || 1, 2)
  width = rect.width
  height = rect.height
  canvas.value.width = Math.max(1, Math.round(width * ratio))
  canvas.value.height = Math.max(1, Math.round(height * ratio))
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0)
  if (reducedMotion) paintStill()
}

function addDrop(x: number, y: number, size: number, alpha: number, life: number) {
  if (drops.length >= MAX_DROPS) drops.shift()
  drops.push({
    x,
    y,
    from: size * 0.45,
    to: size,
    age: 0,
    life,
    alpha,
    sprite: Math.floor(Math.random() * sprites.length),
    angle: random(0, Math.PI * 2)
  })
  start()
}

function drawDrop(drop: Drop, t: number) {
  if (!ctx) return
  const sprite = sprites[drop.sprite]
  if (!sprite) return
  const eased = 1 - Math.pow(1 - t, 3)
  const size = drop.from + (drop.to - drop.from) * eased
  const fade = t < 0.06 ? t / 0.06 : Math.pow(1 - (t - 0.06) / 0.94, 1.4)
  ctx.globalAlpha = drop.alpha * fade * (dark ? 0.55 : 1)
  ctx.save()
  ctx.translate(drop.x, drop.y)
  ctx.rotate(drop.angle)
  ctx.drawImage(sprite, -size, -size, size * 2, size * 2)
  ctx.restore()
}

function tick(time: number) {
  frame = 0
  if (!ctx) return
  const delta = lastTime ? Math.min(time - lastTime, 64) : 16
  lastTime = time
  ctx.clearRect(0, 0, width, height)
  drops = drops.filter((drop) => {
    drop.age += delta
    const t = drop.age / drop.life
    if (t >= 1) return false
    drawDrop(drop, t)
    return true
  })
  ctx.globalAlpha = 1
  if (drops.length) start()
  else lastTime = 0
}

function start() {
  if (frame || !visible || reducedMotion || document.hidden) return
  frame = requestAnimationFrame(tick)
}

function stop() {
  if (frame) cancelAnimationFrame(frame)
  frame = 0
  lastTime = 0
}

function paintStill() {
  if (!ctx) return
  ctx.clearRect(0, 0, width, height)
  const still: Drop[] = [
    { x: width * 0.78, y: height * 0.32, from: 0, to: 150, age: 0, life: 1, alpha: 0.18, sprite: 0, angle: 0.4 },
    { x: width * 0.12, y: height * 0.8, from: 0, to: 90, age: 0, life: 1, alpha: 0.14, sprite: 1, angle: 2 }
  ]
  for (const drop of still) {
    ctx.globalAlpha = drop.alpha * (dark ? 0.55 : 1)
    ctx.drawImage(sprites[drop.sprite], drop.x - drop.to, drop.y - drop.to, drop.to * 2, drop.to * 2)
  }
  ctx.globalAlpha = 1
}

function onPointerMove(event: PointerEvent) {
  if (!host || reducedMotion) return
  const rect = host.getBoundingClientRect()
  const x = event.clientX - rect.left
  const y = event.clientY - rect.top
  if (!lastPoint) {
    lastPoint = { x, y }
    return
  }
  const distance = Math.hypot(x - lastPoint.x, y - lastPoint.y)
  if (distance < 26) return
  // Slow strokes pool more ink; fast strokes leave lighter, larger washes.
  const speed = Math.min(distance / 120, 1)
  addDrop(x, y, random(40, 70) + speed * 50, 0.32 - speed * 0.14, random(2800, 4200))
  lastPoint = { x, y }
}

function onPointerDown(event: PointerEvent) {
  if (!host || reducedMotion) return
  const rect = host.getBoundingClientRect()
  addDrop(event.clientX - rect.left, event.clientY - rect.top, random(110, 150), 0.42, 5200)
}

function onPointerLeave() {
  lastPoint = null
}

function ambientDrop() {
  if (visible && !document.hidden && width > 0) {
    addDrop(random(width * 0.45, width * 0.95), random(height * 0.1, height * 0.7), random(70, 140), 0.14, 6000)
  }
}

function onVisibilityChange() {
  if (document.hidden) stop()
  else if (drops.length) start()
}

onMounted(() => {
  const el = canvas.value
  host = el?.parentElement ?? null
  ctx = el?.getContext?.('2d') ?? null
  if (!el || !host || !ctx) return

  reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  buildSprites()
  resize()

  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(resize)
    resizeObserver.observe(host)
  }
  if (typeof IntersectionObserver !== 'undefined') {
    intersectionObserver = new IntersectionObserver(([entry]) => {
      visible = entry?.isIntersecting ?? true
      if (visible && drops.length) start()
      else if (!visible) stop()
    })
    intersectionObserver.observe(host)
  }
  themeObserver = new MutationObserver(() => {
    const nowDark = document.documentElement.classList.contains('dark')
    if (nowDark === dark) return
    buildSprites()
    if (reducedMotion) paintStill()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })

  if (reducedMotion) return
  host.addEventListener('pointermove', onPointerMove, { passive: true })
  host.addEventListener('pointerdown', onPointerDown, { passive: true })
  host.addEventListener('pointerleave', onPointerLeave, { passive: true })
  document.addEventListener('visibilitychange', onVisibilityChange)
  ambientTimer = window.setInterval(ambientDrop, 2600)
  window.setTimeout(ambientDrop, 400)
})

onBeforeUnmount(() => {
  stop()
  window.clearInterval(ambientTimer)
  resizeObserver?.disconnect()
  intersectionObserver?.disconnect()
  themeObserver?.disconnect()
  host?.removeEventListener('pointermove', onPointerMove)
  host?.removeEventListener('pointerdown', onPointerDown)
  host?.removeEventListener('pointerleave', onPointerLeave)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  drops = []
  sprites = []
})
</script>

<style scoped>
.jack-ink-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
