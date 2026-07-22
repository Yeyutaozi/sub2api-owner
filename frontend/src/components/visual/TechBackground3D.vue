<template>
  <div
    ref="host"
    class="tech-background"
    :class="`tech-background--${props.variant}`"
    aria-hidden="true"
  ></div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ACESFilmicToneMapping,
  BoxGeometry,
  BufferGeometry,
  CatmullRomCurve3,
  Color,
  DirectionalLight,
  DoubleSide,
  DynamicDrawUsage,
  EdgesGeometry,
  Float32BufferAttribute,
  Group,
  HemisphereLight,
  InstancedMesh,
  LineBasicMaterial,
  LineSegments,
  Mesh,
  MeshBasicMaterial,
  MeshStandardMaterial,
  Object3D,
  PerspectiveCamera,
  Points,
  RingGeometry,
  Scene,
  ShaderMaterial,
  SphereGeometry,
  SRGBColorSpace,
  TorusGeometry,
  TubeGeometry,
  Vector3,
  WebGLRenderer
} from 'three'
import type { Material } from 'three'

type SceneVariant = 'home' | 'auth'

interface PacketRig {
  meshes: Mesh[]
  path: CatmullRomCurve3
}

interface HomeOutputRig {
  material: MeshStandardMaterial
  path: CatmullRomCurve3
  port: Group
  portMaterial: MeshStandardMaterial
  returnPath: CatmullRomCurve3
}

interface HomePulseRing {
  material: MeshBasicMaterial
  mesh: Mesh
}

interface AuthProbeFrame {
  line: LineSegments
  material: LineBasicMaterial
}

interface HomeSceneRig {
  accretionMaterial: ShaderMaterial
  diskGroup: Group
  eventHorizon: Mesh
  inputPacket: PacketRig
  motionGroup: Group
  orbitGroups: Group[]
  outputs: HomeOutputRig[]
  outputPacket: PacketRig
  pulseRings: HomePulseRing[]
  returnPacket: PacketRig
  starFields: Points[]
}

interface AuthSceneRig {
  gateCount: number
  gateDepths: number[]
  gateInstances: InstancedMesh
  gateMaterial: MeshStandardMaterial
  matrixHelper: Object3D
  probeFrames: AuthProbeFrame[]
  sealFrame: LineSegments
  sealMaterial: LineBasicMaterial
}

interface DisposableObject extends Object3D {
  geometry?: BufferGeometry
  material?: Material | Material[]
}

const props = withDefaults(defineProps<{ variant?: SceneVariant; routeIndex?: number }>(), {
  variant: 'home',
  routeIndex: 0
})

const host = ref<HTMLDivElement | null>(null)
const compactSceneBreakpoint = 780
const routeColors = [0xef775a, 0x42d5b5, 0x6f91ff, 0xdbe4e2]
const forwardAxis = new Vector3(0, 0, 1)
const authCheckpointColorHex = [0x42d5b5, 0x6f91ff, 0xef775a]
const authCheckpointColors = authCheckpointColorHex.map((color) => new Color(color))
const authSealColors = [new Color(0x42d5b5), new Color(0xef775a)]
const authGateBaseColor = new Color(0x2d4a47)
const authGateColorScratch = new Color()
const instancePositionScratch = new Vector3()
const instanceScaleScratch = new Vector3()

let scene: Scene | undefined
let camera: PerspectiveCamera | undefined
let renderer: WebGLRenderer | undefined
let sceneRoot: Group | undefined
let homeRig: HomeSceneRig | undefined
let authRig: AuthSceneRig | undefined
let animationFrame = 0
let lastFrameAt = 0
let lastSceneTime = 0
let scenePhaseStartedAt = 0
let targetFrameInterval = 1000 / 60
let renderWidth = 0
let renderHeight = 0
let renderPixelRatio = 0
let sceneCompact: boolean | undefined
let isVisible = true
let isDisposed = false
let isContextLost = false
let initializationFailed = false
let initializationRetryCount = 0
let initializationRetryTimer: number | undefined
let motionPreference: MediaQueryList | undefined
let pointerPreference: MediaQueryList | undefined
let resizeObserver: ResizeObserver | undefined
let intersectionObserver: IntersectionObserver | undefined
let pointerTargetX = 0
let pointerTargetY = 0
let pointerCurrentX = 0
let pointerCurrentY = 0
let lastHomeAnimationTime = 0

function clamp01(value: number) {
  return Math.min(1, Math.max(0, value))
}

function smoothStep(value: number) {
  const clamped = clamp01(value)
  return clamped * clamped * (3 - 2 * clamped)
}

function easeInCubic(value: number) {
  const clamped = clamp01(value)
  return clamped * clamped * clamped
}

function easeOutCubic(value: number) {
  const clamped = clamp01(value)
  return 1 - Math.pow(1 - clamped, 3)
}

function createStandardMaterial(color: number, opacity: number, emissiveIntensity = 0.12) {
  return new MeshStandardMaterial({
    color,
    depthWrite: false,
    emissive: color,
    emissiveIntensity,
    metalness: 0.72,
    opacity,
    roughness: 0.28,
    transparent: opacity < 1
  })
}

function createMechanicalMaterial(
  color: number,
  emissive: number = color,
  emissiveIntensity = 0.1
) {
  return new MeshStandardMaterial({
    color,
    depthWrite: true,
    emissive,
    emissiveIntensity,
    metalness: 0.68,
    roughness: 0.34
  })
}

function createPacketRig(path: CatmullRomCurve3, color: number, compact: boolean) {
  const geometry = new SphereGeometry(compact ? 0.085 : 0.11, compact ? 8 : 12, compact ? 5 : 7)
  const segmentCount = compact ? 1 : 3
  const meshes: Mesh[] = []

  for (let index = 0; index < segmentCount; index += 1) {
    const opacity = index === 0 ? 1 : index === 1 ? 0.42 : 0.16
    const material = createStandardMaterial(color, opacity, index === 0 ? 0.82 : 0.24)
    const mesh = new Mesh(geometry, material)
    mesh.scale.set(1, 1, index === 0 ? 2.8 : 2.2 - index * 0.35)
    mesh.visible = false
    meshes.push(mesh)
  }

  return { meshes, path }
}

function addPacketToGroup(packet: PacketRig, group: Group) {
  packet.meshes.forEach((mesh) => group.add(mesh))
}

function placePacket(packet: PacketRig, progress: number, visible: boolean) {
  packet.meshes.forEach((mesh, index) => {
    const travel = progress - index * 0.024
    mesh.visible = visible && travel >= 0 && travel <= 1
    if (!mesh.visible) return

    const point = packet.path.getPointAt(clamp01(travel))
    const tangent = packet.path.getTangentAt(clamp01(travel)).normalize()
    const crossScale = 1 - index * 0.16
    mesh.position.copy(point)
    mesh.quaternion.setFromUnitVectors(forwardAxis, tangent)
    mesh.scale.set(crossScale, crossScale, (index === 0 ? 2.8 : 2.2 - index * 0.35) * crossScale)
  })
}

function setInstanceBar(
  instances: InstancedMesh,
  helper: Object3D,
  index: number,
  position: Vector3,
  scale: Vector3,
  rotationZ = 0
) {
  helper.position.copy(position)
  helper.rotation.set(0, 0, rotationZ)
  helper.scale.copy(scale)
  helper.updateMatrix()
  instances.setMatrixAt(index, helper.matrix)
}

function setAnimatedInstanceBar(
  instances: InstancedMesh,
  helper: Object3D,
  index: number,
  positionX: number,
  positionY: number,
  positionZ: number,
  scaleX: number,
  scaleY: number,
  scaleZ: number,
  color: Color,
  rotationZ = 0
) {
  instancePositionScratch.set(positionX, positionY, positionZ)
  if (rotationZ) instancePositionScratch.applyAxisAngle(forwardAxis, rotationZ)
  instanceScaleScratch.set(scaleX, scaleY, scaleZ)
  setInstanceBar(instances, helper, index, instancePositionScratch, instanceScaleScratch, rotationZ)
  instances.setColorAt(index, color)
}

function createSeededRandom(seed: number) {
  let state = seed >>> 0
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0
    return state / 4294967296
  }
}

function createCosmicStarField(compact: boolean) {
  const random = createSeededRandom(0x243f6a88)
  const count = compact ? 300 : 980
  const positions: number[] = []
  const colors: number[] = []
  const sizes: number[] = []
  const palette = [new Color(0xe8edf4), new Color(0xc6d9f4), new Color(0xffe0bd)]

  for (let index = 0; index < count; index += 1) {
    const x = (random() - 0.5) * 30
    const banded = index % 4 === 0
    const y = banded ? (random() - 0.5) * 0.82 + x * 0.035 : (random() - 0.5) * 16
    const z = -28 + random() * 19
    const paletteRoll = random()
    const color = palette[paletteRoll < 0.82 ? 0 : paletteRoll < 0.94 ? 1 : 2]
    const luminance = 0.58 + random() * 0.42
    const magnitude = Math.pow(random(), 3.2)
    positions.push(x, y, z)
    colors.push(color.r * luminance, color.g * luminance, color.b * luminance)
    sizes.push(0.72 + magnitude * 1.88)
  }

  const geometry = new BufferGeometry()
  geometry.setAttribute('position', new Float32BufferAttribute(positions, 3))
  geometry.setAttribute('color', new Float32BufferAttribute(colors, 3))
  geometry.setAttribute('aSize', new Float32BufferAttribute(sizes, 1))
  const material = new ShaderMaterial({
    depthWrite: false,
    transparent: true,
    vertexShader: `
      attribute float aSize;
      attribute vec3 color;
      varying vec3 vColor;

      void main() {
        vColor = color;
        gl_PointSize = aSize;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      varying vec3 vColor;

      void main() {
        float distanceToCenter = length(gl_PointCoord - vec2(0.5));
        if (distanceToCenter > 0.5) discard;
        float core = 1.0 - smoothstep(0.07, 0.18, distanceToCenter);
        float halo = 1.0 - smoothstep(0.18, 0.5, distanceToCenter);
        gl_FragColor = vec4(vColor, core + halo * 0.34);
      }
    `
  })
  material.toneMapped = false
  const stars = new Points(geometry, material)
  stars.frustumCulled = false
  return stars
}

function createAccretionMaterial(outerRadius: number) {
  const material = new ShaderMaterial({
    depthWrite: false,
    side: DoubleSide,
    transparent: true,
    uniforms: {
      uActiveColor: { value: new Color(routeColors[0]) },
      uInnerColor: { value: new Color(0xfff0cf) },
      uImpact: { value: 0 },
      uMidColor: { value: new Color(0xd66a2c) },
      uOuterRadius: { value: outerRadius },
      uOuterColor: { value: new Color(0x481b18) },
      uTime: { value: 0 }
    },
    vertexShader: `
      varying vec2 vLocal;

      void main() {
        vLocal = position.xy;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform vec3 uActiveColor;
      uniform vec3 uInnerColor;
      uniform vec3 uMidColor;
      uniform float uOuterRadius;
      uniform vec3 uOuterColor;
      uniform float uImpact;
      uniform float uTime;
      varying vec2 vLocal;

      void main() {
        float radius = length(vLocal);
        float angle = atan(vLocal.y, vLocal.x);
        float innerFade = smoothstep(1.02, 1.12, radius);
        float outerFade = 1.0 - smoothstep(uOuterRadius - 0.38, uOuterRadius, radius);
        float radialBand = sin(radius * 72.0 - uTime * 0.22) * 0.1;
        float spiralBand = sin(radius * 31.0 + angle * 7.0 + uTime * 0.11) * 0.08;
        float bandLight = 0.82 + radialBand + spiralBand;
        float midBlend = 1.0 - smoothstep(1.45, 2.92, radius);
        float innerBlend = 1.0 - smoothstep(1.02, 1.48, radius);
        vec3 plasma = mix(uOuterColor, uMidColor, midBlend);
        plasma = mix(plasma, uInnerColor, innerBlend);
        plasma += uActiveColor * (0.035 + uImpact * 0.09);
        float doppler = 0.64 + 0.36 * cos(angle - 0.45);
        float alpha = innerFade * outerFade * (0.48 + midBlend * 0.32);
        gl_FragColor = vec4(plasma * bandLight * doppler, alpha * (0.9 + uImpact * 0.1));
      }
    `
  })
  material.toneMapped = false
  return material
}

function createHomeScene(root: Group, compact: boolean) {
  const motionGroup = new Group()
  root.add(motionGroup)

  const starField = createCosmicStarField(compact)
  scene?.add(starField)

  const inputPath = new CatmullRomCurve3([
    new Vector3(-5.8, -1.15, -1.6),
    new Vector3(-4.1, -0.98, -1.12),
    new Vector3(-2.55, -1.16, -0.46),
    new Vector3(-1.48, -0.62, 0.04),
    new Vector3(-0.78, -0.18, 0.18)
  ])
  const inputMaterial = createStandardMaterial(0x42d5b5, 0.58, 0.44)
  motionGroup.add(new Mesh(
    new TubeGeometry(inputPath, compact ? 28 : 58, compact ? 0.018 : 0.026, compact ? 3 : 5, false),
    inputMaterial
  ))

  const outerDiskRadius = compact ? 2.55 : 3.1
  const accretionMaterial = createAccretionMaterial(outerDiskRadius)
  const diskGroup = new Group()
  diskGroup.rotation.set(1.16, -0.24, -0.2)
  const accretionDisk = new Mesh(
    new RingGeometry(1.02, outerDiskRadius, compact ? 56 : 112, compact ? 4 : 7),
    accretionMaterial
  )
  accretionDisk.renderOrder = 2
  diskGroup.add(accretionDisk)

  const photonMaterial = new MeshBasicMaterial({
    color: 0xffe4bb,
    depthWrite: false,
    opacity: 0.58,
    transparent: true
  })
  photonMaterial.toneMapped = false
  const photonRing = new Mesh(
    new TorusGeometry(compact ? 0.8 : 0.98, compact ? 0.014 : 0.018, 4, compact ? 52 : 96),
    photonMaterial
  )
  photonRing.renderOrder = 4
  diskGroup.add(photonRing)
  motionGroup.add(diskGroup)

  const horizonRadius = compact ? 0.72 : 0.88
  const eventHorizon = new Mesh(
    new SphereGeometry(horizonRadius, compact ? 20 : 32, compact ? 12 : 20),
    new MeshBasicMaterial({ color: 0x000000, depthWrite: true })
  )
  eventHorizon.renderOrder = 3
  motionGroup.add(eventHorizon)

  if (!compact) {
    const lensMaterial = new MeshBasicMaterial({
      color: 0xffeed2,
      depthWrite: false,
      opacity: 0.24,
      transparent: true
    })
    lensMaterial.toneMapped = false
    const lensArc = new Mesh(
      new TorusGeometry(1.08, 0.024, 4, 64, Math.PI * 1.12),
      lensMaterial
    )
    lensArc.position.z = 0.05
    lensArc.rotation.set(1.05, -0.2, 0.2)
    lensArc.renderOrder = 5
    motionGroup.add(lensArc)
  }

  const orbitGroups: Group[] = []
  const orbitSpecs = compact
    ? [{ radius: 3.15, color: 0x88a7ba, x: 0.72, y: 0.18, z: 0.5 }]
    : [
        { radius: 3.55, color: 0x6f91ff, x: 0.68, y: 0.2, z: 0.46 },
        { radius: 4.05, color: 0x7aa89f, x: -0.34, y: 0.72, z: -0.62 },
        { radius: 4.52, color: 0x9a705e, x: 0.42, y: -0.58, z: 1.08 }
      ]
  orbitSpecs.forEach((spec, index) => {
    const group = new Group()
    const material = new MeshBasicMaterial({
      color: spec.color,
      depthWrite: false,
      opacity: compact ? 0.09 : 0.12 - index * 0.018,
      transparent: true
    })
    const guide = new Mesh(
      new TorusGeometry(spec.radius, compact ? 0.009 : 0.012, 3, compact ? 56 : 88, Math.PI * 1.62),
      material
    )
    guide.rotation.z = -0.78 + index * 0.9
    group.rotation.set(spec.x, spec.y, spec.z)
    group.position.z = -0.45 - index * 0.38
    group.add(guide)
    orbitGroups.push(group)
    motionGroup.add(group)
  })

  const beaconRingGeometry = new TorusGeometry(0.16, 0.012, 4, compact ? 18 : 30)
  const beaconCoreGeometry = new SphereGeometry(0.065, compact ? 8 : 12, compact ? 5 : 7)
  const beaconEndpoints = [
    new Vector3(3, 1.52, -1.15),
    new Vector3(3.62, 0.58, -1.85),
    new Vector3(3.48, -0.42, -2.55),
    new Vector3(2.78, -1.26, -3)
  ]
  const outputs: HomeOutputRig[] = beaconEndpoints.map((endpoint, index) => {
    const color = routeColors[index]
    const path = new CatmullRomCurve3([
      new Vector3(-0.78, -0.18, 0.18),
      new Vector3(-0.4, -1.04, 0.08 + index * 0.025),
      new Vector3(0.46, -1.2, -0.04 - index * 0.05),
      new Vector3(1.12, -0.5 + index * 0.18, -0.22 - index * 0.16),
      new Vector3(1.9 + index * 0.1, endpoint.y * 0.74, endpoint.z * 0.68),
      endpoint
    ])
    const material = createStandardMaterial(0xbfe9df, 0.055, 0.12)
    motionGroup.add(new Mesh(
      new TubeGeometry(path, compact ? 28 : 56, compact ? 0.018 : 0.022, compact ? 3 : 5, false),
      material
    ))

    const portMaterial = createMechanicalMaterial(color, color, 0.34)
    const port = new Group()
    const beaconRing = new Mesh(beaconRingGeometry, portMaterial)
    const beaconCore = new Mesh(beaconCoreGeometry, portMaterial)
    port.add(beaconRing, beaconCore)
    const tangent = path.getTangentAt(1).normalize()
    port.position.copy(endpoint)
    port.quaternion.setFromUnitVectors(forwardAxis, tangent)
    motionGroup.add(port)

    const returnPath = new CatmullRomCurve3([
      endpoint,
      new Vector3(1.62, -1.86, -2.18 - index * 0.12),
      new Vector3(-0.4, -2.12, -2.42),
      new Vector3(-4.55, -1.72, -2.8)
    ])
    return { material, path, port, portMaterial, returnPath }
  })

  const inputPacket = createPacketRig(inputPath, 0x42d5b5, compact)
  const outputPacket = createPacketRig(outputs[0].path, routeColors[0], compact)
  const returnPacket = createPacketRig(outputs[0].returnPath, 0xeef7f5, compact)
  addPacketToGroup(inputPacket, motionGroup)
  addPacketToGroup(outputPacket, motionGroup)
  addPacketToGroup(returnPacket, motionGroup)

  const pulseRings: HomePulseRing[] = []
  const pulseRingCount = compact ? 1 : 3
  for (let index = 0; index < pulseRingCount; index += 1) {
    const material = new MeshBasicMaterial({
      color: 0xffead0,
      depthWrite: false,
      opacity: 0,
      transparent: true
    })
    material.toneMapped = false
    const mesh = new Mesh(
      new TorusGeometry(0.96, compact ? 0.01 : 0.014, 4, compact ? 42 : 68),
      material
    )
    mesh.rotation.copy(diskGroup.rotation)
    mesh.renderOrder = 6 + index
    mesh.visible = false
    pulseRings.push({ material, mesh })
    motionGroup.add(mesh)
  }

  homeRig = {
    accretionMaterial,
    diskGroup,
    eventHorizon,
    inputPacket,
    motionGroup,
    orbitGroups,
    outputs,
    outputPacket,
    pulseRings,
    returnPacket,
    starFields: [starField]
  }
}

function createAuthScene(root: Group, compact: boolean) {
  const gateCount = compact ? 4 : 7
  const gateDepths = Array.from({ length: gateCount }, (_, index) => -6.4 + index * (7.2 / Math.max(1, gateCount - 1)))
  const gateInstanceCount = gateCount * 4 + 4
  const gateMaterial = new MeshStandardMaterial({
    color: 0xffffff,
    depthWrite: true,
    emissive: 0x0a1716,
    emissiveIntensity: 0.2,
    metalness: 0.72,
    opacity: 0.92,
    roughness: 0.32,
    transparent: true,
    vertexColors: true
  })
  const gateInstances = new InstancedMesh(new BoxGeometry(1, 1, 1), gateMaterial, gateInstanceCount)
  gateInstances.instanceMatrix.setUsage(DynamicDrawUsage)
  for (let index = 0; index < gateInstanceCount; index += 1) {
    gateInstances.setColorAt(index, authGateBaseColor)
  }
  gateInstances.instanceColor?.setUsage(DynamicDrawUsage)
  const matrixHelper = new Object3D()
  root.add(gateInstances)

  const probeSource = new BoxGeometry(6.26, 5.62, 0.08)
  const probeGeometry = new EdgesGeometry(probeSource)
  probeSource.dispose()
  const probeFrames: AuthProbeFrame[] = []
  const probeFrameCount = compact ? 2 : 3
  for (let index = 0; index < probeFrameCount; index += 1) {
    const material = new LineBasicMaterial({ color: 0x42d5b5, opacity: 0, transparent: true })
    const line = new LineSegments(probeGeometry, material)
    line.visible = false
    probeFrames.push({ line, material })
    root.add(line)
  }

  const sealSource = new BoxGeometry(6.72, 6.04, 0.1)
  const sealMaterial = new LineBasicMaterial({ color: 0xeef7f5, opacity: 0, transparent: true })
  const sealFrame = new LineSegments(new EdgesGeometry(sealSource), sealMaterial)
  sealSource.dispose()
  sealFrame.visible = false
  root.add(sealFrame)

  authRig = {
    gateCount,
    gateDepths,
    gateInstances,
    gateMaterial,
    matrixHelper,
    probeFrames,
    sealFrame,
    sealMaterial
  }
}

function createSceneGeometry(root: Group, compact: boolean) {
  if (props.variant === 'home') {
    createHomeScene(root, compact)
    return
  }

  const hemisphere = new HemisphereLight(0xbfd4ff, 0x03100e, 1.25)
  const mintLight = new DirectionalLight(0x42d5b5, 2.6)
  const blueLight = new DirectionalLight(0x6f91ff, 2.1)
  const coralLight = new DirectionalLight(0xef775a, 1.25)
  mintLight.position.set(-4, 5, 7)
  blueLight.position.set(4, -2, 6)
  coralLight.position.set(5, 4, 2)
  root.add(hemisphere, mintLight, blueLight, coralLight)

  createAuthScene(root, compact)
}

function updateLayout() {
  const element = host.value
  if (!element || !renderer || !camera || !sceneRoot) return

  const bounds = element.getBoundingClientRect()
  const width = Math.max(1, Math.round(bounds.width))
  const height = Math.max(1, Math.round(bounds.height))
  const compact = width <= compactSceneBreakpoint
  const pixelRatioLimit = compact ? 1 : props.variant === 'home' ? 1.25 : 1.5
  const pixelRatio = Math.min(window.devicePixelRatio || 1, pixelRatioLimit)
  targetFrameInterval = 1000 / (compact ? 30 : 60)

  if (sceneCompact !== undefined && sceneCompact !== compact) {
    disposeScene()
    initializeScene()
    return
  }

  if (width !== renderWidth || height !== renderHeight || pixelRatio !== renderPixelRatio) {
    renderer.setPixelRatio(pixelRatio)
    renderer.setSize(width, height, false)
    renderWidth = width
    renderHeight = height
    renderPixelRatio = pixelRatio
  }
  camera.aspect = width / height
  const narrowHome = props.variant === 'home' && width <= 520
  const mediumHome = props.variant === 'home' && width > 520 && width < 1200
  camera.fov = props.variant === 'home'
    ? narrowHome ? 48 : mediumHome ? 44 : 42
    : compact ? 46 : 42
  const homeCameraZ = narrowHome ? 10.8 : mediumHome ? 10.2 : 9.8
  camera.position.set(0, 0, props.variant === 'home' ? homeCameraZ : compact ? 12.4 : 11.2)
  camera.lookAt(props.variant === 'home' ? 0.12 : 0, props.variant === 'home' ? -0.02 : 0, props.variant === 'home' ? -1.05 : -0.55)
  camera.updateProjectionMatrix()

  if (props.variant === 'home') {
    const rootX = width <= 520 ? 0.72 : width < 1200 ? 1.55 : 2.15
    const rootY = width <= 520 ? 2.55 : width < 1200 ? 2.05 : 2
    const rootZ = width <= 520 ? -0.6 : width < 1200 ? -0.45 : -0.35
    const rootScale = width <= 520 ? 0.58 : width < 1200 ? 0.72 : 0.86
    sceneRoot.position.set(rootX, rootY, rootZ)
    sceneRoot.scale.setScalar(rootScale)
  } else {
    sceneRoot.position.set(0, compact ? -0.22 : -0.5, -0.25)
    sceneRoot.scale.setScalar(compact ? 0.82 : 1)
  }

  renderScene(lastSceneTime)
}

function setPacketColor(packet: PacketRig, color: number) {
  packet.meshes.forEach((mesh, index) => {
    const material = mesh.material as MeshStandardMaterial
    material.color.setHex(color)
    material.emissive.setHex(color)
    material.emissiveIntensity = index === 0 ? 0.82 : 0.24
  })
}

function animateHome(time: number) {
  if (!homeRig || !sceneRoot || !camera) return

  const phase = (time % 5.6) / 5.6
  const compact = sceneCompact ?? false
  const deltaTime = lastHomeAnimationTime > 0 && time >= lastHomeAnimationTime
    ? Math.min(0.05, time - lastHomeAnimationTime)
    : 1 / 60
  lastHomeAnimationTime = time
  const parallaxActive = !compact && Boolean(pointerPreference?.matches) && !motionPreference?.matches
  if (!parallaxActive) {
    pointerTargetX = 0
    pointerTargetY = 0
  }
  const pointerDamping = 1 - Math.exp(-deltaTime * 6)
  pointerCurrentX += (pointerTargetX - pointerCurrentX) * pointerDamping
  pointerCurrentY += (pointerTargetY - pointerCurrentY) * pointerDamping
  const activeIndex = Math.abs(props.routeIndex) % homeRig.outputs.length
  const activeOutput = homeRig.outputs[activeIndex]
  const inboundDolly = phase <= 0.3
    ? easeInCubic(phase / 0.3)
    : phase < 0.5
      ? 1 - smoothStep((phase - 0.3) / 0.2)
      : 0
  const slingshotProgress = clamp01((phase - 0.3) / 0.22)
  const slingshot = phase >= 0.3 && phase <= 0.52 ? Math.sin(slingshotProgress * Math.PI) : 0
  const branchPower = smoothStep((phase - 0.48) / 0.1)
  const endpointLock = smoothStep((phase - 0.56) / 0.1)
  const responseProgress = clamp01((phase - 0.7) / 0.14)
  const responseKick = phase >= 0.7 && phase <= 0.84 ? Math.sin(responseProgress * Math.PI) : 0

  const cameraBaseZ = renderWidth <= 520 ? 10.8 : renderWidth < 1200 ? 10.2 : 9.8
  const cameraScale = compact ? 0.5 : 1
  const beaconLook = branchPower * (1 - smoothStep((phase - 0.72) / 0.12))
  camera.position.set(
    -0.12 + pointerCurrentX * 0.18 + slingshot * 0.055 * cameraScale,
    0.06 - pointerCurrentY * 0.11 + activeOutput.port.position.y * 0.012 * beaconLook,
    cameraBaseZ - inboundDolly * 0.24 * cameraScale + slingshot * 0.08 * cameraScale + responseKick * 0.04 * cameraScale
  )
  camera.lookAt(
    0.12 + pointerCurrentX * 0.035 + slingshot * 0.018,
    -0.02 - pointerCurrentY * 0.022 + activeOutput.port.position.y * 0.01 * beaconLook,
    -1.05
  )
  camera.rotateZ((slingshot * 0.005 - responseKick * 0.002) * cameraScale)

  sceneRoot.rotation.set(0, 0, 0)
  homeRig.motionGroup.position.set(slingshot * 0.025, 0, 0)
  homeRig.motionGroup.rotation.set(0, 0, 0)
  homeRig.accretionMaterial.uniforms.uTime.value = time
  homeRig.accretionMaterial.uniforms.uImpact.value = slingshot
  const activeDiskColor = homeRig.accretionMaterial.uniforms.uActiveColor.value as Color
  activeDiskColor.setHex(routeColors[activeIndex])
  homeRig.diskGroup.rotation.set(
    1.16 + Math.sin(time * 0.08) * 0.006,
    -0.24 + Math.sin(time * 0.06) * 0.006,
    -0.2 + Math.sin(time * 0.08) * 0.008
  )
  homeRig.diskGroup.scale.setScalar(1 + slingshot * 0.018)
  homeRig.eventHorizon.scale.setScalar(1 + slingshot * 0.014)
  homeRig.starFields.forEach((field, index) => {
    field.rotation.y = time * (0.0026 + index * 0.0004)
    field.rotation.z = -0.035 + Math.sin(time * 0.025) * 0.004
  })
  homeRig.orbitGroups.forEach((group, index) => {
    const guide = group.children[0]
    if (guide) guide.rotation.z = -0.78 + index * 0.9 + time * (index % 2 === 0 ? 0.018 : -0.014)
  })

  placePacket(homeRig.inputPacket, easeInCubic(phase / 0.3), phase < 0.315)

  homeRig.outputs.forEach((output, index) => {
    const active = index === activeIndex
    output.material.opacity = active ? 0.055 + branchPower * 0.765 : compact ? 0.018 : 0.035
    output.material.emissiveIntensity = active ? 0.12 + branchPower * 0.86 : 0.035
    output.portMaterial.color.setHex(active ? routeColors[index] : 0x10191d)
    output.portMaterial.emissive.setHex(routeColors[index])
    output.portMaterial.emissiveIntensity = active ? 0.22 + endpointLock * 0.72 : 0.035
    const portPulse = active && phase >= 0.54 && phase <= 0.7
      ? 1 + Math.sin(smoothStep((phase - 0.54) / 0.16) * Math.PI) * 0.11
      : 1
    output.port.scale.setScalar(portPulse)
  })

  homeRig.outputPacket.path = activeOutput.path
  setPacketColor(homeRig.outputPacket, 0xbfe9df)
  placePacket(homeRig.outputPacket, smoothStep((phase - 0.31) / 0.36), phase >= 0.31 && phase <= 0.69)

  homeRig.returnPacket.path = activeOutput.returnPath
  placePacket(homeRig.returnPacket, smoothStep((phase - 0.7) / 0.14), phase >= 0.7 && phase <= 0.86)

  homeRig.pulseRings.forEach(({ mesh, material }, index) => {
    const pulseStart = 0.3 + index * 0.024
    const pulseDuration = 0.18 + index * 0.016
    const pulseProgress = clamp01((phase - pulseStart) / pulseDuration)
    mesh.visible = phase >= pulseStart && phase <= pulseStart + pulseDuration
    if (!mesh.visible) return

    mesh.scale.setScalar(0.92 + easeOutCubic(pulseProgress) * (0.62 + index * 0.14))
    material.opacity = (1 - pulseProgress) * Math.max(0.18, 0.44 - index * 0.1)
  })
}

function animateAuth(time: number) {
  if (!authRig || !sceneRoot || !camera) return

  const phase = (time % 8.4) / 8.4
  const compact = sceneCompact ?? false
  const probeProgress = smoothStep((phase - 0.18) / 0.38)
  const probeZ = -6.8 + probeProgress * 7.85
  const probeActive = phase >= 0.16 && phase <= 0.66
  const sealSlide = smoothStep((phase - 0.56) / 0.06)
  const sealSettle = smoothStep((phase - 0.62) / 0.045)
  const sealHitProgress = clamp01((phase - 0.56) / 0.11)
  const sealHit = phase >= 0.56 && phase <= 0.67 ? Math.sin(sealHitProgress * Math.PI) : 0
  const cameraReturn = phase < 0.56
    ? probeProgress
    : phase < 0.72
      ? 1 - smoothStep((phase - 0.56) / 0.16)
      : 0

  const cameraScale = compact ? 0.5 : 1
  camera.position.set(
    sealHit * 0.028 * cameraScale,
    -sealHit * 0.016 * cameraScale,
    (compact ? 12.4 : 11.2) - cameraReturn * 0.14 * cameraScale + sealHit * 0.11 * cameraScale
  )
  camera.lookAt(0, 0, -0.55)
  camera.rotateZ(sealHit * 0.005 * cameraScale)

  sceneRoot.rotation.x = -0.006 + probeProgress * 0.012 - sealHit * 0.018
  sceneRoot.rotation.y = (probeProgress - 0.5) * 0.014 + sealHit * 0.02

  const probeColor = phase < 0.32
    ? authCheckpointColorHex[0]
    : phase < 0.44
      ? authCheckpointColorHex[1]
      : phase < 0.56
        ? authCheckpointColorHex[2]
        : 0xeef7f5
  const probeOpacity = phase < 0.18
    ? clamp01((phase - 0.16) / 0.02) * 0.86
    : phase < 0.56
      ? 0.9
      : 0.9 * (1 - clamp01((phase - 0.56) / 0.1))
  for (let index = 0; index < authRig.probeFrames.length; index += 1) {
    const { line, material } = authRig.probeFrames[index]
    line.visible = probeActive && probeOpacity > 0
    line.position.z = probeZ - index * 0.42
    line.rotation.z = (index % 2 === 0 ? -1 : 1) * index * 0.006
    line.scale.setScalar(1 + index * 0.024)
    material.color.setHex(probeColor)
    material.opacity = probeOpacity * (index === 0 ? 1 : index === 1 ? 0.34 : 0.14)
  }

  let instanceIndex = 0
  for (let gateIndex = 0; gateIndex < authRig.gateDepths.length; gateIndex += 1) {
    const depth = authRig.gateDepths[gateIndex]
    const distance = Math.abs(probeZ - depth)
    const active = probeActive ? Math.exp(-distance * distance * 3.2) : 0
    const completed = probeActive && probeZ > depth ? 0.22 : 0
    const width = 6.64 - active * 0.74 - completed * 0.08
    const height = 5.98 - active * 0.64 - completed * 0.07
    const thickness = 0.065 + active * 0.13
    const gateZ = depth + active * 0.26
    const twist = (gateIndex % 2 === 0 ? -1 : 1) * active * 0.045
    const checkpointColor = authCheckpointColors[Math.min(2, Math.floor((gateIndex / authRig.gateCount) * 3))]
    authGateColorScratch.copy(authGateBaseColor).lerp(
      checkpointColor,
      Math.min(1, active + completed)
    )

    setAnimatedInstanceBar(
      authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
      0, height / 2, gateZ, width, thickness, 0.13, authGateColorScratch, twist
    )
    setAnimatedInstanceBar(
      authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
      0, -height / 2, gateZ, width, thickness, 0.13, authGateColorScratch, twist
    )
    setAnimatedInstanceBar(
      authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
      width / 2, 0, gateZ, thickness, height, 0.13, authGateColorScratch, twist
    )
    setAnimatedInstanceBar(
      authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
      -width / 2, 0, gateZ, thickness, height, 0.13, authGateColorScratch, twist
    )
  }

  const sealWidth = 6.72
  const sealHeight = 6.04
  const sealThickness = 0.095
  const sealOffset = (1 - sealSlide) * 1.34 - Math.sin(sealSettle * Math.PI) * 0.1
  const sealStretch = phase < 0.56 ? 0 : 0.56 + sealSlide * 0.48 - sealSettle * 0.04
  setAnimatedInstanceBar(
    authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
    0, sealHeight / 2 + sealOffset, 1.02,
    sealWidth * sealStretch, sealThickness, 0.16, authSealColors[0]
  )
  setAnimatedInstanceBar(
    authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
    0, -sealHeight / 2 - sealOffset, 1.02,
    sealWidth * sealStretch, sealThickness, 0.16, authSealColors[1]
  )
  setAnimatedInstanceBar(
    authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
    sealWidth / 2 + sealOffset, 0, 1.02,
    sealThickness, sealHeight * sealStretch, 0.16, authSealColors[0]
  )
  setAnimatedInstanceBar(
    authRig.gateInstances, authRig.matrixHelper, instanceIndex++,
    -sealWidth / 2 - sealOffset, 0, 1.02,
    sealThickness, sealHeight * sealStretch, 0.16, authSealColors[1]
  )

  authRig.gateInstances.instanceMatrix.needsUpdate = true
  if (authRig.gateInstances.instanceColor) authRig.gateInstances.instanceColor.needsUpdate = true
  authRig.gateMaterial.opacity = phase >= 0.68 ? 0.84 : 0.92
  authRig.gateMaterial.emissiveIntensity = phase >= 0.56 ? 0.3 : 0.2

  const sealFlash = clamp01((phase - 0.605) / 0.065)
  authRig.sealFrame.visible = phase >= 0.605
  authRig.sealFrame.position.z = 1.02
  authRig.sealFrame.scale.setScalar(0.9 + easeOutCubic(sealFlash) * 0.2)
  authRig.sealMaterial.color.setHex(phase <= 0.67 ? 0xeef7f5 : 0xef775a)
  authRig.sealMaterial.opacity = phase <= 0.67 ? Math.sin(sealFlash * Math.PI) * 0.94 : 0.42
}

function renderScene(time: number) {
  if (!renderer || !scene || !camera || !sceneRoot) return

  lastSceneTime = time
  if (props.variant === 'home') animateHome(time)
  else animateAuth(time)
  renderer.render(scene, camera)
}

function canAnimate() {
  return isVisible && !document.hidden && !motionPreference?.matches && !isDisposed && !isContextLost
}

function stopAnimation() {
  if (animationFrame) {
    window.cancelAnimationFrame(animationFrame)
    animationFrame = 0
  }
}

function scheduleAnimation() {
  if (!animationFrame && renderer && canAnimate()) {
    animationFrame = window.requestAnimationFrame(renderFrame)
  }
}

function renderFrame(timestamp: number) {
  animationFrame = 0
  if (!canAnimate()) return

  const elapsed = timestamp - lastFrameAt
  if (elapsed >= targetFrameInterval - 0.75) {
    const remainder = elapsed >= targetFrameInterval ? elapsed % targetFrameInterval : 0
    lastFrameAt = timestamp - remainder
    if (!scenePhaseStartedAt) scenePhaseStartedAt = timestamp
    renderScene((timestamp - scenePhaseStartedAt) / 1000)
  }
  scheduleAnimation()
}

function resetPointerParallax(immediate = false) {
  pointerTargetX = 0
  pointerTargetY = 0
  if (immediate) {
    pointerCurrentX = 0
    pointerCurrentY = 0
  }
}

function handlePointerMove(event: PointerEvent) {
  if (
    props.variant !== 'home' ||
    sceneCompact !== false ||
    !pointerPreference?.matches ||
    motionPreference?.matches
  ) {
    resetPointerParallax()
    return
  }

  pointerTargetX = Math.max(-1, Math.min(1, (event.clientX / window.innerWidth) * 2 - 1))
  pointerTargetY = Math.max(-1, Math.min(1, (event.clientY / window.innerHeight) * 2 - 1))
}

function handlePointerLeave() {
  resetPointerParallax()
}

function handleVisibilityChange() {
  if (document.hidden) {
    resetPointerParallax(true)
    stopAnimation()
  }
  else scheduleAnimation()
}

function handleMotionChange() {
  resetPointerParallax(true)
  if (motionPreference?.matches) {
    disposeScene()
    return
  }

  initializationFailed = false
  initializationRetryCount = 0
  initializeScene()
}

function handleContextLost(event: Event) {
  event.preventDefault()
  isContextLost = true
  stopAnimation()
}

function handleContextRestored() {
  isContextLost = false
  updateLayout()
  scheduleAnimation()
}

function initializeScene() {
  const element = host.value
  if (!element || renderer || initializationFailed || isDisposed) return

  try {
    isVisible = true
    isContextLost = false
    if (!scenePhaseStartedAt) scenePhaseStartedAt = performance.now()
    const compact = element.getBoundingClientRect().width <= compactSceneBreakpoint
    sceneCompact = compact
    scene = new Scene()
    camera = new PerspectiveCamera(42, 1, 0.1, 80)
    sceneRoot = new Group()
    scene.add(sceneRoot)
    createSceneGeometry(sceneRoot, compact)

    renderer = new WebGLRenderer({
      alpha: true,
      antialias: !compact,
      powerPreference: compact ? 'low-power' : 'high-performance'
    })
    renderer.outputColorSpace = SRGBColorSpace
    renderer.toneMapping = ACESFilmicToneMapping
    renderer.toneMappingExposure = compact ? 1.06 : 1.12
    renderer.setClearColor(0x000000, 0)
    renderer.domElement.addEventListener('webglcontextlost', handleContextLost)
    renderer.domElement.addEventListener('webglcontextrestored', handleContextRestored)
    element.appendChild(renderer.domElement)

    const ResizeObserverConstructor = window.ResizeObserver as typeof ResizeObserver | undefined
    if (ResizeObserverConstructor) {
      resizeObserver = new ResizeObserverConstructor(updateLayout)
      resizeObserver.observe(element)
    } else {
      window.addEventListener('resize', updateLayout)
    }

    const IntersectionObserverConstructor = window.IntersectionObserver as typeof IntersectionObserver | undefined
    if (IntersectionObserverConstructor) {
      intersectionObserver = new IntersectionObserverConstructor(([entry]) => {
        isVisible = entry?.isIntersecting ?? true
        if (isVisible) scheduleAnimation()
        else stopAnimation()
      }, { threshold: 0.01 })
      intersectionObserver.observe(element)
    }

    updateLayout()
    scheduleAnimation()
    initializationRetryCount = 0
  } catch {
    initializationFailed = true
    disposeScene()
    if (initializationRetryCount < 1 && !isDisposed && !motionPreference?.matches) {
      initializationRetryCount += 1
      initializationRetryTimer = window.setTimeout(() => {
        initializationRetryTimer = undefined
        initializationFailed = false
        initializeScene()
      }, 300)
    }
  }
}

function disposeScene() {
  stopAnimation()
  resetPointerParallax(true)
  lastHomeAnimationTime = 0
  if (initializationRetryTimer !== undefined) {
    window.clearTimeout(initializationRetryTimer)
    initializationRetryTimer = undefined
  }
  resizeObserver?.disconnect()
  intersectionObserver?.disconnect()
  resizeObserver = undefined
  intersectionObserver = undefined
  window.removeEventListener('resize', updateLayout)

  const disposedGeometries = new Set<BufferGeometry>()
  const disposedMaterials = new Set<Material>()
  scene?.traverse((object) => {
    if (object instanceof InstancedMesh) object.dispose()
    const disposable = object as DisposableObject
    if (disposable.geometry && !disposedGeometries.has(disposable.geometry)) {
      disposable.geometry.dispose()
      disposedGeometries.add(disposable.geometry)
    }

    const materials = Array.isArray(disposable.material) ? disposable.material : [disposable.material]
    materials.forEach((material) => {
      if (material && !disposedMaterials.has(material)) {
        material.dispose()
        disposedMaterials.add(material)
      }
    })
  })

  if (renderer) {
    renderer.domElement.removeEventListener('webglcontextlost', handleContextLost)
    renderer.domElement.removeEventListener('webglcontextrestored', handleContextRestored)
    renderer.dispose()
    renderer.forceContextLoss()
    renderer.domElement.remove()
  }

  scene = undefined
  camera = undefined
  renderer = undefined
  sceneRoot = undefined
  homeRig = undefined
  authRig = undefined
  renderWidth = 0
  renderHeight = 0
  renderPixelRatio = 0
  sceneCompact = undefined
  isContextLost = false
}

onMounted(() => {
  scenePhaseStartedAt = performance.now()
  motionPreference = window.matchMedia('(prefers-reduced-motion: reduce)')
  pointerPreference = window.matchMedia('(hover: hover) and (pointer: fine)')
  motionPreference.addEventListener('change', handleMotionChange)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  if (props.variant === 'home') {
    window.addEventListener('pointermove', handlePointerMove, { passive: true })
    window.addEventListener('blur', handlePointerLeave)
    document.documentElement.addEventListener('pointerleave', handlePointerLeave)
  }
  if (!motionPreference.matches) initializeScene()
})

onBeforeUnmount(() => {
  isDisposed = true
  motionPreference?.removeEventListener('change', handleMotionChange)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('blur', handlePointerLeave)
  document.documentElement.removeEventListener('pointerleave', handlePointerLeave)
  disposeScene()
})

watch(() => props.routeIndex, () => {
  scenePhaseStartedAt = performance.now()
  lastSceneTime = 0
  if (canAnimate()) renderScene(0)
})
</script>

<style scoped>
.tech-background {
  position: absolute;
  inset: 0;
  overflow: hidden;
  contain: strict;
  pointer-events: none;
}

.tech-background--home { opacity: 0.96; }
.tech-background--auth { opacity: 0.82; }

.tech-background :deep(canvas) {
  display: block;
  width: 100%;
  height: 100%;
}

@media (max-width: 780px) {
  .tech-background--home { opacity: 0.66; }
  .tech-background--auth { opacity: 0.62; }
}

@media (prefers-reduced-motion: reduce) {
  .tech-background { display: none; }
}
</style>
