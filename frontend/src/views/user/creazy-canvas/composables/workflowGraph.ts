export interface WorkflowGraphNode {
  id: string
}

export type WorkflowGraphNodeInput = string | WorkflowGraphNode

export interface WorkflowGraphEdge {
  id?: string | null
  source: string
  target: string
}

export interface TopologicalOrderResult {
  order: string[]
  hasCycle: boolean
  /** Nodes that cannot be ordered because they are in, or depend on, a cycle. */
  unresolvedNodeIds: string[]
}

function uniqueNodeIds(nodes: Iterable<WorkflowGraphNodeInput>): string[] {
  const ids: string[] = []
  const seen = new Set<string>()

  for (const node of nodes) {
    const id = typeof node === 'string' ? node : node.id
    if (seen.has(id)) continue
    seen.add(id)
    ids.push(id)
  }

  return ids
}

function insertByNodeOrder(queue: string[], nodeId: string, rank: ReadonlyMap<string, number>): void {
  const nodeRank = rank.get(nodeId) ?? Number.MAX_SAFE_INTEGER
  const index = queue.findIndex((queuedId) => (rank.get(queuedId) ?? Number.MAX_SAFE_INTEGER) > nodeRank)
  if (index === -1) {
    queue.push(nodeId)
    return
  }
  queue.splice(index, 0, nodeId)
}

/**
 * Checks whether adding source -> target would close a directed cycle.
 * Edges outside the provided node set are ignored. When reconnecting an edge,
 * pass its id so the previous connection is removed from the check.
 */
export function wouldCreateCycle(
  nodes: Iterable<WorkflowGraphNodeInput>,
  edges: readonly WorkflowGraphEdge[],
  source: string,
  target: string,
  ignoreEdgeId?: string,
): boolean {
  if (source === target) return true

  const nodeIds = new Set(uniqueNodeIds(nodes))
  if (!nodeIds.has(source) || !nodeIds.has(target)) return false

  const adjacency = new Map<string, string[]>()
  for (const nodeId of nodeIds) adjacency.set(nodeId, [])

  for (const edge of edges) {
    if (ignoreEdgeId !== undefined && edge.id === ignoreEdgeId) continue
    if (!nodeIds.has(edge.source) || !nodeIds.has(edge.target)) continue
    adjacency.get(edge.source)?.push(edge.target)
  }

  const pending = [target]
  const visited = new Set<string>()

  while (pending.length > 0) {
    const nodeId = pending.pop()
    if (nodeId === undefined || visited.has(nodeId)) continue
    if (nodeId === source) return true
    visited.add(nodeId)
    pending.push(...(adjacency.get(nodeId) || []))
  }

  return false
}

/**
 * Produces a stable topological order using the input node order as the tie
 * breaker. Only edges internal to nodeIds participate in the sort.
 */
export function topologicalOrder(
  nodeIds: Iterable<string>,
  edges: readonly WorkflowGraphEdge[],
): TopologicalOrderResult {
  const ids = uniqueNodeIds(nodeIds)
  const idSet = new Set(ids)
  const rank = new Map(ids.map((id, index) => [id, index]))
  const indegree = new Map(ids.map((id) => [id, 0]))
  const adjacency = new Map(ids.map((id) => [id, [] as string[]]))

  for (const edge of edges) {
    if (!idSet.has(edge.source) || !idSet.has(edge.target)) continue
    adjacency.get(edge.source)?.push(edge.target)
    indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1)
  }

  const ready = ids.filter((id) => indegree.get(id) === 0)
  const order: string[] = []

  while (ready.length > 0) {
    const nodeId = ready.shift()
    if (nodeId === undefined) break
    order.push(nodeId)

    for (const targetId of adjacency.get(nodeId) || []) {
      const nextIndegree = (indegree.get(targetId) || 0) - 1
      indegree.set(targetId, nextIndegree)
      if (nextIndegree === 0) insertByNodeOrder(ready, targetId, rank)
    }
  }

  const orderedIds = new Set(order)
  const unresolvedNodeIds = ids.filter((id) => !orderedIds.has(id))

  return {
    order,
    hasCycle: unresolvedNodeIds.length > 0,
    unresolvedNodeIds,
  }
}

/**
 * Returns every node reachable from start, excluding start itself. allowedIds
 * restricts both traversal and output to an induced subgraph when provided.
 */
export function downstreamNodeIds(
  start: string,
  edges: readonly WorkflowGraphEdge[],
  allowedIds?: Iterable<string>,
): string[] {
  const allowed = allowedIds ? new Set(allowedIds) : null
  if (allowed && !allowed.has(start)) return []

  const adjacency = new Map<string, string[]>()
  for (const edge of edges) {
    if (allowed && (!allowed.has(edge.source) || !allowed.has(edge.target))) continue
    const targets = adjacency.get(edge.source)
    if (targets) targets.push(edge.target)
    else adjacency.set(edge.source, [edge.target])
  }

  const result: string[] = []
  const visited = new Set([start])
  const pending = [...(adjacency.get(start) || [])]

  for (let index = 0; index < pending.length; index += 1) {
    const nodeId = pending[index]
    if (nodeId === undefined || visited.has(nodeId)) continue
    visited.add(nodeId)
    result.push(nodeId)
    pending.push(...(adjacency.get(nodeId) || []))
  }

  return result
}

/** Returns edges whose source and target are both selected. */
export function internalEdges<T extends WorkflowGraphEdge>(
  selectedIds: Iterable<string>,
  edges: readonly T[],
): T[] {
  const selected = new Set(selectedIds)
  return edges.filter((edge) => selected.has(edge.source) && selected.has(edge.target))
}
