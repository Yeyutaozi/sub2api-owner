import { describe, expect, it } from 'vitest'
import {
  downstreamNodeIds,
  internalEdges,
  topologicalOrder,
  wouldCreateCycle,
  type WorkflowGraphEdge,
} from '../workflowGraph'

function edge(source: string, target: string, id = `${source}-${target}`): WorkflowGraphEdge {
  return { id, source, target }
}

describe('wouldCreateCycle', () => {
  it('detects a path back to the proposed source and rejects self edges', () => {
    const nodes = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
    const edges = [edge('a', 'b'), edge('b', 'c')]

    expect(wouldCreateCycle(nodes, edges, 'c', 'a')).toBe(true)
    expect(wouldCreateCycle(nodes, edges, 'a', 'c')).toBe(false)
    expect(wouldCreateCycle(nodes, edges, 'b', 'b')).toBe(true)
  })

  it('ignores the old edge while validating a reconnection', () => {
    const nodes = ['a', 'b']
    const edges = [edge('a', 'b', 'rewired-edge')]

    expect(wouldCreateCycle(nodes, edges, 'b', 'a')).toBe(true)
    expect(wouldCreateCycle(nodes, edges, 'b', 'a', 'rewired-edge')).toBe(false)
  })

  it('does not let orphan edges outside the known graph affect the result', () => {
    const nodes = ['a', 'b']
    const edges = [edge('outside', 'a'), edge('b', 'outside')]

    expect(wouldCreateCycle(nodes, edges, 'a', 'b')).toBe(false)
  })
})

describe('topologicalOrder', () => {
  it('orders isolated nodes and a multi-branch DAG deterministically', () => {
    const result = topologicalOrder(
      ['solo', 'a', 'b', 'c', 'd'],
      [edge('a', 'b'), edge('a', 'c'), edge('b', 'd'), edge('c', 'd')],
    )

    expect(result).toEqual({
      order: ['solo', 'a', 'b', 'c', 'd'],
      hasCycle: false,
      unresolvedNodeIds: [],
    })
  })

  it('reports a cycle while retaining the sortable prefix', () => {
    const result = topologicalOrder(
      ['isolated', 'a', 'b', 'blocked'],
      [edge('a', 'b'), edge('b', 'a'), edge('b', 'blocked')],
    )

    expect(result.order).toEqual(['isolated'])
    expect(result.hasCycle).toBe(true)
    expect(result.unresolvedNodeIds).toEqual(['a', 'b', 'blocked'])
  })

  it('sorts an induced subgraph and ignores all external edges', () => {
    const result = topologicalOrder(
      ['b', 'c', 'd'],
      [edge('a', 'b'), edge('b', 'c'), edge('c', 'd'), edge('d', 'outside')],
    )

    expect(result).toEqual({
      order: ['b', 'c', 'd'],
      hasCycle: false,
      unresolvedNodeIds: [],
    })
  })

  it('deduplicates node ids without changing their first-seen order', () => {
    expect(topologicalOrder(['a', 'a', 'b'], [edge('a', 'b')]).order).toEqual(['a', 'b'])
  })
})

describe('downstreamNodeIds', () => {
  const edges = [
    edge('a', 'b'),
    edge('a', 'c'),
    edge('b', 'd'),
    edge('c', 'd'),
    edge('d', 'a'),
    edge('x', 'y'),
  ]

  it('returns every reachable node once and excludes the start node in a cycle', () => {
    expect(downstreamNodeIds('a', edges)).toEqual(['b', 'c', 'd'])
  })

  it('returns an empty list for an isolated node', () => {
    expect(downstreamNodeIds('isolated', edges)).toEqual([])
  })

  it('restricts traversal to the induced allowed subgraph', () => {
    expect(downstreamNodeIds('a', edges, ['a', 'b', 'd'])).toEqual(['b', 'd'])
    expect(downstreamNodeIds('a', [edge('a', 'x'), edge('x', 'd')], ['a', 'd'])).toEqual([])
    expect(downstreamNodeIds('outside', edges, ['a', 'b'])).toEqual([])
  })
})

describe('internalEdges', () => {
  it('returns only edges fully contained by the selected subgraph', () => {
    const edges = [
      edge('a', 'b'),
      edge('b', 'c'),
      edge('c', 'd'),
      edge('d', 'e'),
      edge('b', 'd'),
    ]

    expect(internalEdges(['b', 'c', 'd'], edges)).toEqual([edges[1], edges[2], edges[4]])
  })

  it('returns an empty list for an empty selection', () => {
    expect(internalEdges([], [edge('a', 'b')])).toEqual([])
  })
})
