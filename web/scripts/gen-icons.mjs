// Erzeugt die PNG-Icons (PWA, Apple) aus derselben Geometrie wie public/favicon.svg.
// Ohne Abhängigkeiten: einfache Rasterung über Abstandsfunktionen + PNG über zlib.
// Aufruf: node scripts/gen-icons.mjs
import { writeFileSync } from 'node:fs'
import { deflateSync } from 'node:zlib'

const BG = [0xe4, 0xe4, 0xe7] // zinc-200
const FG = [0x18, 0x18, 0x1b] // zinc-900

// Abstand eines Punkts zu einem abgerundeten Rechteck (negativ = innen).
function roundRect(px, py, x, y, w, h, r) {
  const cx = x + w / 2
  const cy = y + h / 2
  const qx = Math.abs(px - cx) - w / 2 + r
  const qy = Math.abs(py - cy) - h / 2 + r
  return Math.hypot(Math.max(qx, 0), Math.max(qy, 0)) + Math.min(Math.max(qx, qy), 0) - r
}

// Motiv im 32er-Raster wie favicon.svg: drei volle Kacheln, eine als Umriss.
function glyph(px, py) {
  const solid = Math.min(
    roundRect(px, py, 7, 7, 8, 8, 2),
    roundRect(px, py, 17, 7, 8, 8, 2),
    roundRect(px, py, 7, 17, 8, 8, 2),
  )
  const outline = Math.abs(roundRect(px, py, 18.1, 18.1, 5.8, 5.8, 1.4)) - 1.1
  return Math.min(solid, outline)
}

function render(size, { rounded }) {
  const scale = size / 32
  const rows = []
  for (let y = 0; y < size; y++) {
    const row = Buffer.alloc(1 + size * 4) // Filterbyte 0 + RGBA
    for (let x = 0; x < size; x++) {
      const px = (x + 0.5) / scale
      const py = (y + 0.5) / scale
      // Deckung aus dem Abstand in Pixeln (Kantenglättung über 1 px)
      const cover = (d) => Math.min(1, Math.max(0, 0.5 - d * scale))
      const bgA = rounded ? cover(roundRect(px, py, 0, 0, 32, 32, 7)) : 1
      const fgA = cover(glyph(px, py))
      const o = 1 + x * 4
      for (let c = 0; c < 3; c++) row[o + c] = Math.round(BG[c] * (1 - fgA) + FG[c] * fgA)
      row[o + 3] = Math.round(255 * Math.max(bgA, fgA))
    }
    rows.push(row)
  }
  return png(size, Buffer.concat(rows))
}

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
  return c >>> 0
})
function crc32(buf) {
  let c = 0xffffffff
  for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}
function chunk(type, data) {
  const len = Buffer.alloc(4)
  len.writeUInt32BE(data.length)
  const body = Buffer.concat([Buffer.from(type), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(body))
  return Buffer.concat([len, body, crc])
}
function png(size, raw) {
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(size, 0)
  ihdr.writeUInt32BE(size, 4)
  ihdr[8] = 8 // Bittiefe
  ihdr[9] = 6 // RGBA
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ])
}

const out = (name) => new URL(`../public/icons/${name}`, import.meta.url)
writeFileSync(out('icon-192.png'), render(192, { rounded: true }))
writeFileSync(out('icon-512.png'), render(512, { rounded: true }))
// Maskable und Apple: vollflächig, das System rundet selbst ab (Motiv liegt in der sicheren Zone).
writeFileSync(out('icon-maskable-512.png'), render(512, { rounded: false }))
writeFileSync(out('apple-touch-icon.png'), render(180, { rounded: false }))
console.log('icons ok')
