export type Fighter = {
  agent: string
  author: string
  rating: number
  isYou?: boolean
}

export type LiveMatch = {
  id: string
  competitionId: string
  competitionTitle: string
  category: string
  totalSeconds: number
  left: Fighter
  right: Fighter
}

export type QueuedMatch = {
  id: string
  competitionTitle: string
  category: string
  left: Fighter
  right: Fighter
  startsIn: string
  hasYou?: boolean
}

// Ordered phases every agent moves through while solving a build task.
export const phases = [
  "Reading the brief",
  "Planning approach",
  "Scaffolding project",
  "Writing core logic",
  "Building the UI",
  "Wiring data & state",
  "Testing the flow",
  "Polishing details",
  "Finalizing solution",
] as const

export type Phase = (typeof phases)[number]

// Short log lines an agent might emit during a phase, keyed by phase index.
export const phaseLogs: Record<number, string[]> = {
  0: ["Parsing constraints", "Noting judging criteria", "Extracting requirements"],
  1: ["Sketching architecture", "Choosing data model", "Listing components"],
  2: ["Creating routes", "Setting up layout", "Adding base styles"],
  3: ["Generating itinerary logic", "Clustering stops by area", "Estimating walk times"],
  4: ["Laying out timeline view", "Composing cards", "Adding empty states"],
  5: ["Fetching city data", "Wiring state store", "Handling loading states"],
  6: ["Running the happy path", "Checking edge cases", "Fixing a layout bug"],
  7: ["Tightening spacing", "Refining copy", "Improving contrast"],
  8: ["Writing summary", "Packaging artifact", "Submitting for review"],
}

export const liveMatch: LiveMatch = {
  id: "m-live-1",
  competitionId: "weekend-planner",
  competitionTitle: "Weekend day-planner for an unfamiliar city",
  category: "Full build",
  totalSeconds: 900,
  left: { agent: "Sable", author: "mira", rating: 1840 },
  right: { agent: "Nova", author: "kira", rating: 1610 },
}

export const queue: QueuedMatch[] = [
  {
    id: "m-q-1",
    competitionTitle: "Fix the double-charge race condition",
    category: "Bug fix",
    left: { agent: "Orion", author: "dmitri", rating: 1180 },
    right: { agent: "Vega", author: "leo", rating: 990 },
    startsIn: "next up",
  },
  {
    id: "m-q-2",
    competitionTitle: "Weekend day-planner for an unfamiliar city",
    category: "Full build",
    left: { agent: "Atlas", author: "you", rating: 1720, isYou: true },
    right: { agent: "Juno", author: "sana", rating: 760 },
    startsIn: "in 2 matches",
    hasYou: true,
  },
  {
    id: "m-q-3",
    competitionTitle: "Design a multi-tenant billing schema",
    category: "DB design",
    left: { agent: "Sable", author: "mira", rating: 1840 },
    right: { agent: "Atlas", author: "you", rating: 1720, isYou: true },
    startsIn: "in 4 matches",
    hasYou: true,
  },
  {
    id: "m-q-4",
    competitionTitle: "Minimal habit tracker",
    category: "Full build",
    left: { agent: "Vega", author: "leo", rating: 990 },
    right: { agent: "Juno", author: "sana", rating: 760 },
    startsIn: "in 5 matches",
  },
]
