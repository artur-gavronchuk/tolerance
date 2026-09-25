'use client'

import { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { eventsBetween, snapshotAt, SLOT_COLORS, type Snapshot } from '@/lib/tanks/playback'
import type { Clock } from '@/lib/tanks/playback'
import type { Replay } from '@/lib/tanks/replay'

// Engine coordinates are (x, y) on a 60x40 field, y up, angles measured
// CCW from +x. Three's field sits on the XZ plane (y is height): engine
// +x stays three +x, engine +y (north) becomes three -z, and everything is
// shifted so the field center (30, 20) lands on the three origin. A
// direction vector at engine angle θ, (cos θ, sin θ), maps to three
// (cos θ, -sin θ) in (x, z) — which is exactly what three's own
// `rotation.y = θ` produces for an object whose local forward is +x, so
// the hull/turret groups below use the engine angle unmodified.
const FIELD_W = 60
const FIELD_H = 40
function toThree(x: number, y: number): [number, number] {
  return [x - FIELD_W / 2, -(y - FIELD_H / 2)]
}

// Kill explosions are aged in simulation ticks against the replay's own
// clock (same pattern as Canvas2D's shot/hit/kill effects), not wall time,
// so they hold still while paused and scale with playback speed instead of
// drifting relative to it. ~0.5s at the usual 10 ticks/s tick rate.
const KILL_LIFE_TICKS = 5.2

interface KillFx {
  bornTick: number
  mesh: THREE.Mesh
}

interface TankRig {
  root: THREE.Group
  hull: THREE.Mesh
  turretPivot: THREE.Group
  hpBg: THREE.Sprite
  hpFg: THREE.Sprite
}

const HP_BAR_W = 1.6
const HP_BAR_H = 0.16

function makeTank(color: THREE.ColorRepresentation): TankRig {
  const root = new THREE.Group()

  const hullGeo = new THREE.BoxGeometry(2.0, 0.6, 1.4)
  const hullMat = new THREE.MeshStandardMaterial({ color, roughness: 0.6, metalness: 0.15 })
  const hull = new THREE.Mesh(hullGeo, hullMat)
  hull.position.y = 0.3
  hull.castShadow = true
  hull.receiveShadow = true
  root.add(hull)

  const turretPivot = new THREE.Group()
  turretPivot.position.y = 0.6
  root.add(turretPivot)

  const turretGeo = new THREE.CylinderGeometry(0.5, 0.5, 0.4, 16)
  const turretMat = new THREE.MeshStandardMaterial({ color, roughness: 0.5, metalness: 0.2 })
  const turret = new THREE.Mesh(turretGeo, turretMat)
  turret.position.y = 0.2
  turret.castShadow = true
  turretPivot.add(turret)

  const barrelGeo = new THREE.CylinderGeometry(0.1, 0.1, 1.3, 12)
  const barrelMat = new THREE.MeshStandardMaterial({ color: '#2a2f33', roughness: 0.4, metalness: 0.4 })
  const barrel = new THREE.Mesh(barrelGeo, barrelMat)
  barrel.rotation.z = Math.PI / 2
  barrel.position.set(0.65, 0.2, 0)
  barrel.castShadow = true
  turretPivot.add(barrel)

  const hpBg = new THREE.Sprite(new THREE.SpriteMaterial({ color: '#0c1720', depthTest: false, transparent: true, opacity: 0.85 }))
  hpBg.scale.set(HP_BAR_W, HP_BAR_H, 1)
  hpBg.position.y = 1.35
  hpBg.renderOrder = 10
  root.add(hpBg)

  const hpFg = new THREE.Sprite(new THREE.SpriteMaterial({ color, depthTest: false, transparent: true, opacity: 0.95 }))
  hpFg.scale.set(HP_BAR_W, HP_BAR_H * 0.72, 1)
  hpFg.position.y = 1.35
  hpFg.renderOrder = 11
  root.add(hpFg)

  return { root, hull, turretPivot, hpBg, hpFg }
}

function disposeMesh(mesh: THREE.Mesh | THREE.Sprite) {
  mesh.geometry?.dispose()
  const mat = mesh.material
  if (Array.isArray(mat)) mat.forEach((m) => m.dispose())
  else mat?.dispose()
}

// The 3D battlefield renderer — the same { replay, clock } contract as
// Canvas2D, so the viewer toggle can swap renderers without resetting
// playback. Like Canvas2D, it owns its own requestAnimationFrame loop only
// while the clock is playing, renders one frame on pause/seek/resize via
// Clock.subscribe, and disposes every three.js resource it created on
// unmount.
export default function Scene3D({ replay, clock, selectedSlot, onSelect }: {
  replay: Replay
  clock: Clock
  selectedSlot?: number | null
  onSelect?: (slot: number | null) => void
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const selectedSlotRef = useRef<number | null | undefined>(selectedSlot)
  // Set by the scene effect below to a function that redraws one frame —
  // used so switching who's followed repaints immediately even while the
  // clock is paused, without tearing down and rebuilding the whole scene
  // (the scene effect intentionally does not depend on selectedSlot).
  const requestRenderRef = useRef<() => void>(() => {})
  const [unsupported, setUnsupported] = useState(false)

  useEffect(() => {
    selectedSlotRef.current = selectedSlot
    requestRenderRef.current()
  }, [selectedSlot])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false })
    } catch {
      setUnsupported(true)
      return
    }
    if (!renderer.getContext()) {
      setUnsupported(true)
      renderer.dispose()
      return
    }

    const scene = new THREE.Scene()
    scene.background = new THREE.Color('#0c1720')

    const camera = new THREE.PerspectiveCamera(50, 1, 0.1, 300)
    camera.position.set(0, 42, 46)

    renderer.domElement.style.display = 'block'
    renderer.domElement.style.width = '100%'
    renderer.domElement.style.height = '100%'
    renderer.domElement.style.touchAction = 'none'
    container.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    // Damping is deliberately off: it needs a continuous rAF loop to settle
    // inertia, which conflicts with "the render loop only runs while the
    // Clock is playing" — a paused viewer would otherwise never stop
    // rendering while the user is free to orbit. Without damping, each
    // pointer move fires exactly one 'change' event, handled below by a
    // single render() call, which keeps the same play/pause-driven loop
    // model Canvas2D uses.
    controls.minDistance = 8
    controls.maxDistance = 110
    controls.maxPolarAngle = Math.PI / 2 - 0.02
    controls.target.set(0, 0, 0)
    controls.update()

    // Lights
    const hemi = new THREE.HemisphereLight('#bcd7e8', '#0c1720', 0.9)
    scene.add(hemi)
    const sun = new THREE.DirectionalLight('#fff3d8', 1.1)
    sun.position.set(-24, 34, 18)
    sun.target.position.set(0, 0, 0)
    scene.add(sun)
    scene.add(sun.target)

    // Ground + grid
    const groundGeo = new THREE.PlaneGeometry(FIELD_W, FIELD_H)
    const groundMat = new THREE.MeshStandardMaterial({ color: '#0f1b24', roughness: 0.95, metalness: 0 })
    const ground = new THREE.Mesh(groundGeo, groundMat)
    ground.rotation.x = -Math.PI / 2
    ground.receiveShadow = true
    scene.add(ground)

    const grid = new THREE.GridHelper(FIELD_W, 30, '#2a3a46', '#1a2732')
    grid.scale.z = FIELD_H / FIELD_W
    grid.position.y = 0.005
    scene.add(grid)

    // Walls
    const wallGroup = new THREE.Group()
    const wallGeoCache = new Map<string, THREE.BoxGeometry>()
    const wallMat = new THREE.MeshStandardMaterial({ color: '#2c3b46', roughness: 0.85 })
    for (const w of replay.walls) {
      const key = `${w.w}x${w.h}`
      let geo = wallGeoCache.get(key)
      if (!geo) {
        geo = new THREE.BoxGeometry(w.w, 1.5, w.h)
        wallGeoCache.set(key, geo)
      }
      const mesh = new THREE.Mesh(geo, wallMat)
      const [tx, tz] = toThree(w.x + w.w / 2, w.y + w.h / 2)
      mesh.position.set(tx, 0.75, tz)
      mesh.castShadow = true
      mesh.receiveShadow = true
      wallGroup.add(mesh)
    }
    scene.add(wallGroup)

    // Zone (shrinking safe circle): translucent open cylinder, hidden until
    // it actually starts shrinking below the full-field radius.
    const zoneGeo = new THREE.CylinderGeometry(1, 1, 3, 48, 1, true)
    const zoneMat = new THREE.MeshBasicMaterial({ color: '#ee6b62', transparent: true, opacity: 0.16, side: THREE.DoubleSide, depthWrite: false })
    const zoneMesh = new THREE.Mesh(zoneGeo, zoneMat)
    zoneMesh.position.y = 1.5
    zoneMesh.visible = false
    scene.add(zoneMesh)

    // Tanks
    const tankRigs: TankRig[] = replay.players.map((_, i) => makeTank(SLOT_COLORS[i % SLOT_COLORS.length]))
    for (const rig of tankRigs) scene.add(rig.root)

    // Shells: glowing spheres, one shared geometry, pooled meshes added and
    // removed as the snapshot's shell list changes.
    const shellGeo = new THREE.SphereGeometry(0.2, 12, 12)
    const shellMat = new THREE.MeshStandardMaterial({ color: '#ffd873', emissive: '#ffb454', emissiveIntensity: 1.4 })
    const shellPool: THREE.Mesh[] = []
    function shellMesh(): THREE.Mesh {
      return shellPool.pop() ?? new THREE.Mesh(shellGeo, shellMat)
    }
    const activeShells: THREE.Mesh[] = []

    // Bonuses: a rotating green cross per pickup, built once per snapshot
    // slot index (pool by index since bonus count is small and changes
    // rarely) and rotated as a function of sim tick so it freezes on pause.
    const crossGroupGeo = { bar: new THREE.BoxGeometry(0.9, 0.16, 0.22) }
    const crossMat = new THREE.MeshStandardMaterial({ color: '#2e7d5b', emissive: '#1c5b40', emissiveIntensity: 0.5 })
    function makeCross(): THREE.Group {
      const g = new THREE.Group()
      const barA = new THREE.Mesh(crossGroupGeo.bar, crossMat)
      const barB = new THREE.Mesh(crossGroupGeo.bar, crossMat)
      barB.rotation.y = Math.PI / 2
      g.add(barA, barB)
      g.position.y = 0.5
      return g
    }
    const crossPool: THREE.Group[] = []
    const activeCrosses: THREE.Group[] = []

    // Kill explosions
    const explosionGeo = new THREE.SphereGeometry(0.5, 16, 16)
    const kills: KillFx[] = []
    let lastTick = clock.now()

    function collectEffects(t: number) {
      if (t < lastTick) {
        for (const k of kills) { scene.remove(k.mesh); disposeMesh(k.mesh) }
        kills.length = 0
      }
      const evs = eventsBetween(replay, lastTick, t)
      lastTick = t
      for (const e of evs) {
        if (e.e === 'kill' && e.b != null) {
          const s = snapshotAt(replay, e.t)
          const tank = s.tanks[e.b]
          if (!tank) continue
          const mat = new THREE.MeshBasicMaterial({ color: '#ffb454', transparent: true, opacity: 0.9 })
          const mesh = new THREE.Mesh(explosionGeo, mat)
          const [tx, tz] = toThree(tank.x, tank.y)
          mesh.position.set(tx, 0.6, tz)
          scene.add(mesh)
          kills.push({ bornTick: e.t, mesh })
        }
      }
    }

    function applyShadows(enabled: boolean) {
      renderer.shadowMap.enabled = enabled
      sun.castShadow = enabled
      ground.receiveShadow = enabled
      for (const rig of tankRigs) {
        rig.hull.castShadow = enabled
      }
      for (const w of wallGroup.children) (w as THREE.Mesh).castShadow = enabled
    }

    let cssW = 0
    let cssH = 0
    let shadowsOn = false

    function resize() {
      const rect = container!.getBoundingClientRect()
      cssW = Math.max(1, Math.round(rect.width))
      cssH = Math.max(1, Math.round(rect.height))
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      renderer.setPixelRatio(dpr)
      renderer.setSize(cssW, cssH, false)
      camera.aspect = cssW / cssH
      camera.updateProjectionMatrix()
      const wantShadows = cssW >= 640
      if (wantShadows !== shadowsOn) {
        shadowsOn = wantShadows
        applyShadows(shadowsOn)
      }
      render()
    }
    const ro = new ResizeObserver(resize)
    ro.observe(container)

    let updatingControls = false
    let wasFollowing = false
    const overviewPosition = camera.position.clone()
    function render() {
      if (cssW === 0) return
      const t = clock.now()
      collectEffects(t)
      const snap: Snapshot = snapshotAt(replay, t)
      const rules = replay.rules

      // Tanks
      snap.tanks.forEach((tank, i) => {
        const rig = tankRigs[i]
        if (!rig) return
        const [tx, tz] = toThree(tank.x, tank.y)
        rig.root.position.set(tx, 0, tz)
        rig.hull.rotation.set(0, 0, 0)
        rig.root.rotation.y = tank.hull
        rig.turretPivot.rotation.y = tank.turret - tank.hull
        const mat = rig.hull.material as THREE.MeshStandardMaterial
        if (tank.alive) {
          mat.color.set(SLOT_COLORS[i % SLOT_COLORS.length])
          rig.hull.rotation.z = 0
        } else {
          mat.color.set('#3a4650')
          rig.hull.rotation.z = 0.22
        }
        rig.root.visible = true
        const frac = Math.max(0, Math.min(1, tank.hp / rules.max_hp))
        rig.hpFg.scale.x = HP_BAR_W * frac
        rig.hpFg.position.x = (HP_BAR_W / 2) * (frac - 1)
        rig.hpBg.visible = tank.alive
        rig.hpFg.visible = tank.alive
        rig.turretPivot.visible = tank.alive
      })

      // Shells
      for (const m of activeShells) { scene.remove(m); shellPool.push(m) }
      activeShells.length = 0
      for (const s of snap.shells) {
        const m = shellMesh()
        const [sx, sz] = toThree(s.x, s.y)
        m.position.set(sx, 0.5, sz)
        scene.add(m)
        activeShells.push(m)
      }

      // Bonuses
      for (const c of activeCrosses) { scene.remove(c); crossPool.push(c) }
      activeCrosses.length = 0
      for (const b of snap.bonuses) {
        const c = crossPool.pop() ?? makeCross()
        const [bx, bz] = toThree(b.x, b.y)
        c.position.x = bx
        c.position.z = bz
        c.rotation.y = t * 0.12
        scene.add(c)
        activeCrosses.push(c)
      }

      // Zone
      if (snap.zone < rules.zone_start_radius - 0.01 && snap.zone > 0.01) {
        zoneMesh.visible = true
        zoneMesh.scale.set(snap.zone, 1, snap.zone)
      } else {
        zoneMesh.visible = false
      }

      // Kill explosions, aged in simulation ticks
      for (let i = kills.length - 1; i >= 0; i--) {
        const fx = kills[i]
        const age = t - fx.bornTick
        if (age > KILL_LIFE_TICKS || age < 0) {
          scene.remove(fx.mesh)
          disposeMesh(fx.mesh)
          kills.splice(i, 1)
          continue
        }
        const frac = age / KILL_LIFE_TICKS
        const scale = 0.4 + frac * 4.5
        fx.mesh.scale.setScalar(scale)
        ;(fx.mesh.material as THREE.MeshBasicMaterial).opacity = Math.max(0, 0.9 * (1 - frac))
      }

      // Camera
      const followSlot = selectedSlotRef.current
      if (followSlot != null && snap.tanks[followSlot]) {
        const tank = snap.tanks[followSlot]
        const [tx, tz] = toThree(tank.x, tank.y)
        const behindDist = 6.5
        const camX = tx - Math.cos(tank.hull) * behindDist
        const camZ = tz + Math.sin(tank.hull) * behindDist
        camera.position.set(camX, 3.4, camZ)
        camera.lookAt(tx, 0.7, tz)
        controls.enabled = false
        wasFollowing = true
      } else {
        // Coming back from Follow: the camera was just teleported to a
        // chase-cam pose that has nothing to do with OrbitControls' own
        // remembered orbit, so restore the last Overview pose explicitly
        // rather than feeding update() that arbitrary position (which is
        // also what made update() clamp-and-dispatch-'change' unpredictably
        // right after a release).
        if (wasFollowing) {
          camera.position.copy(overviewPosition)
          controls.target.set(0, 0, 0)
          wasFollowing = false
        }
        controls.enabled = true
        // update() can synchronously dispatch its own 'change' event (e.g.
        // right after leaving Follow, when the camera was just teleported
        // to the chase-cam pose and its distance from the field-center
        // target gets clamped back into [minDistance, maxDistance]) —
        // that event re-enters render() via onControlsChange below. The
        // reentrancy guard breaks that immediately instead of relying on
        // the position settling within OrbitControls' epsilon, which
        // floating-point noise around a clamp boundary isn't guaranteed
        // to do.
        if (!updatingControls) {
          updatingControls = true
          controls.update()
          updatingControls = false
        }
        overviewPosition.copy(camera.position)
      }

      renderer.render(scene, camera)
    }

    let raf = 0
    function loop() {
      render()
      raf = requestAnimationFrame(loop)
    }
    function start() {
      if (raf) return
      raf = requestAnimationFrame(loop)
    }
    function stop() {
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    }

    const unsubscribe = clock.subscribe(() => {
      if (clock.isPlaying()) start()
      else { stop(); render() }
    })

    // Orbiting (mouse drag or touch) while paused needs its own redraw:
    // the clock isn't ticking so nothing else triggers one. Since damping
    // is off, controls.update() inside render() never itself produces a
    // further 'change' event, so this can't recurse — one drag step means
    // one 'change' means one render().
    const onControlsChange = () => { if (!clock.isPlaying()) render() }
    controls.addEventListener('change', onControlsChange)

    requestRenderRef.current = () => { if (!clock.isPlaying()) render() }

    resize()
    if (clock.isPlaying()) start()

    return () => {
      requestRenderRef.current = () => {}
      stop()
      ro.disconnect()
      unsubscribe()
      controls.removeEventListener('change', onControlsChange)
      controls.dispose()

      for (const k of kills) disposeMesh(k.mesh)
      for (const rig of tankRigs) {
        disposeMesh(rig.hull)
        disposeMesh(rig.hpBg)
        disposeMesh(rig.hpFg)
        for (const child of rig.turretPivot.children) disposeMesh(child as THREE.Mesh)
      }
      for (const geo of wallGeoCache.values()) geo.dispose()
      wallMat.dispose()
      // Shell meshes share one geometry and one material across the whole
      // pool (only their position differs), so those are disposed once
      // here rather than per mesh.
      shellGeo.dispose()
      shellMat.dispose()
      crossGroupGeo.bar.dispose()
      crossMat.dispose()
      explosionGeo.dispose()
      zoneGeo.dispose()
      zoneMat.dispose()
      groundGeo.dispose()
      groundMat.dispose()

      if (renderer.domElement.parentElement === container) container.removeChild(renderer.domElement)
      renderer.dispose()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [replay, clock])

  const names = replay.players
  const followedName = selectedSlot != null ? names.find((p) => p.slot === selectedSlot)?.name : null

  return (
    <div className="relative w-full overflow-hidden rounded-[14px] bg-[#0c1720]" style={{ aspectRatio: '3 / 2' }}>
      <div ref={containerRef} className="absolute inset-0" />
      {followedName && (
        <button
          type="button"
          onClick={() => onSelect?.(null)}
          className="absolute left-2.5 top-2.5 rounded-full bg-black/55 px-3 py-1 text-xs font-semibold text-white backdrop-blur hover:bg-black/70"
        >
          Following {followedName} · click to release
        </button>
      )}
      {unsupported && (
        <div className="absolute inset-0 flex items-center justify-center bg-[#0c1720] px-6 text-center text-sm text-muted-foreground">
          Your browser doesn&apos;t support WebGL, so 3D isn&apos;t available. Switch back to the 2D view.
        </div>
      )}
    </div>
  )
}
