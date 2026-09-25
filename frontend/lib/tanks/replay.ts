import { apiRaw } from '@/lib/api'

// Mirrors backend/internal/games/tanks/{rules,maps,game,replay}.go's JSON
// shapes exactly — this is the replay format written by both the server
// and `arena tanks play`, so the field names and units matter.

export interface Rules {
  width: number
  height: number
  tick_rate: number
  ticks: number
  substeps: number
  tank_radius: number
  tank_speed: number
  tank_reverse_speed: number
  hull_turn_rate: number
  turret_turn_rate: number
  reload_ticks: number
  muzzle_offset: number
  shell_speed: number
  shell_lifetime_ticks: number
  shell_damage: number
  max_hp: number
  heal_amount: number
  heal_respawn_ticks: number
  pickup_radius: number
  zone_start_tick: number
  zone_end_tick: number
  zone_start_radius: number
  zone_end_radius: number
  zone_damage_per_tick: number
}

export interface Wall { x: number; y: number; w: number; h: number }

export interface ReplayPlayer {
  slot: number
  name: string
  bot_id?: string
  version?: number
  house: boolean
  source?: string
}

// One recorded tick. k: per-slot [x, y, hull, turret, hp, reload, alive(0|1)].
// s: active shells [id, owner, x, y]. b: active bonus pickups [x, y].
export interface Frame {
  t: number
  k: number[][]
  s: number[][]
  b: number[][]
  z: number
}

// e is "shot" | "hit" | "kill" | "heal"; a is the acting tank's slot
// (-1 for "kill" means the shrinking zone did it); b is the target slot,
// d is the damage or heal amount, both present only when relevant.
export interface ReplayEvent {
  t: number
  e: string
  a: number
  b?: number
  d?: number
}

export interface PlayerResult {
  slot: number
  place: number
  kills: number
  damage: number
  death_tick: number | null
  status: string
}

export interface Replay {
  version: 1
  engine: string
  seed: number
  map: string
  tick_rate: number
  rules: Rules
  walls: Wall[]
  players: ReplayPlayer[]
  frames: Frame[]
  events: ReplayEvent[]
  result: PlayerResult[]
}

// Decompresses a gzip byte stream to the JSON replay it holds. Used by
// both fetchReplay (server response) and parseReplayFile (a local file).
async function inflateJSON(stream: ReadableStream<Uint8Array>): Promise<Replay> {
  const text = await new Response(stream.pipeThrough(new DecompressionStream('gzip'))).text()
  return JSON.parse(text) as Replay
}

export async function fetchReplay(matchId: string): Promise<Replay> {
  const res = await apiRaw(`/tanks/matches/${matchId}/replay`)
  const stream = res.body ?? (await res.blob()).stream()
  return inflateJSON(stream)
}

// Replay files are either plain JSON (as `arena tanks play` writes by
// default) or gzip-compressed JSON (the site's downloads, or a manually
// gzipped file). Sniff the gzip magic bytes rather than trusting the
// filename, since either extension can carry either encoding.
export async function parseReplayFile(file: File): Promise<Replay> {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  const isGzip = bytes.length > 2 && bytes[0] === 0x1f && bytes[1] === 0x8b
  if (isGzip) return inflateJSON(new Blob([buf]).stream())
  return JSON.parse(new TextDecoder().decode(buf)) as Replay
}
