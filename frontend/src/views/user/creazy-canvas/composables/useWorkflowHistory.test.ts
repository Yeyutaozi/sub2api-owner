import { describe, expect, it, vi } from 'vitest'
import { useWorkflowHistory, type WorkflowCommand } from './useWorkflowHistory'

function createCounterCommand(
  label: string,
  counter: { value: number },
  amount = 1,
): WorkflowCommand {
  return {
    label,
    redo: () => {
      counter.value += amount
    },
    undo: () => {
      counter.value -= amount
    },
  }
}

describe('useWorkflowHistory', () => {
  it('executes commands and moves them between undo and redo stacks', async () => {
    const counter = { value: 0 }
    const history = useWorkflowHistory()

    await history.execute(createCounterCommand('Add node', counter))

    expect(counter.value).toBe(1)
    expect(history.canUndo.value).toBe(true)
    expect(history.undoLabel.value).toBe('Add node')

    await expect(history.undo()).resolves.toBe(true)
    expect(counter.value).toBe(0)
    expect(history.canRedo.value).toBe(true)
    expect(history.redoLabel.value).toBe('Add node')

    await expect(history.redo()).resolves.toBe(true)
    expect(counter.value).toBe(1)
    expect(history.canRedo.value).toBe(false)
  })

  it('records actions that have already happened without replaying them', async () => {
    const counter = { value: 3 }
    const command = createCounterCommand('Move nodes', counter, 3)
    const redoSpy = vi.spyOn(command, 'redo')
    const history = useWorkflowHistory()

    await history.record(command)

    expect(counter.value).toBe(3)
    expect(redoSpy).not.toHaveBeenCalled()
    await history.undo()
    expect(counter.value).toBe(0)
    await history.redo()
    expect(counter.value).toBe(3)
  })

  it('keeps at most one hundred undo commands', async () => {
    const undone: number[] = []
    const history = useWorkflowHistory()

    for (let index = 0; index < 105; index += 1) {
      await history.record({
        label: `Command ${index}`,
        redo: () => undefined,
        undo: () => {
          undone.push(index)
        },
      })
    }

    expect(history.undoLabel.value).toBe('Command 104')
    for (let index = 0; index < 100; index += 1) {
      await history.undo()
    }

    expect(undone).toHaveLength(100)
    expect(undone[undone.length - 1]).toBe(5)
    await expect(history.undo()).resolves.toBe(false)
  })

  it('clears redo history after a new command is executed or recorded', async () => {
    const counter = { value: 0 }
    const history = useWorkflowHistory()

    await history.execute(createCounterCommand('First', counter))
    await history.undo()
    expect(history.canRedo.value).toBe(true)

    await history.execute(createCounterCommand('Second', counter))
    expect(history.canRedo.value).toBe(false)

    await history.undo()
    await history.record(createCounterCommand('Already applied', counter))
    expect(history.canRedo.value).toBe(false)
  })

  it('serializes asynchronous commands and continues after a rejected command', async () => {
    const events: string[] = []
    let releaseFirst: (() => void) | undefined
    const history = useWorkflowHistory()

    const first = history.execute({
      label: 'First',
      redo: async () => {
        events.push('first:start')
        await new Promise<void>((resolve) => {
          releaseFirst = resolve
        })
        events.push('first:end')
      },
      undo: () => undefined,
    })
    const failed = history.execute({
      label: 'Failed',
      redo: async () => {
        events.push('failed')
        throw new Error('redo failed')
      },
      undo: () => undefined,
    })
    const third = history.execute({
      label: 'Third',
      redo: () => {
        events.push('third')
      },
      undo: () => undefined,
    })

    expect(events).toEqual(['first:start'])
    expect(history.isBusy.value).toBe(true)
    releaseFirst?.()

    await first
    await expect(failed).rejects.toThrow('redo failed')
    await third

    expect(events).toEqual(['first:start', 'first:end', 'failed', 'third'])
    expect(history.undoLabel.value).toBe('Third')
    expect(history.isBusy.value).toBe(false)
  })

  it('leaves both stacks unchanged when execute, undo, or redo fails', async () => {
    const history = useWorkflowHistory()
    const executeError = new Error('execute failed')

    await expect(history.execute({
      label: 'Broken execute',
      redo: () => {
        throw executeError
      },
      undo: () => undefined,
    })).rejects.toBe(executeError)
    expect(history.canUndo.value).toBe(false)

    let undoShouldFail = true
    let redoShouldFail = false
    const command: WorkflowCommand = {
      label: 'Recoverable',
      redo: () => {
        if (redoShouldFail) throw new Error('redo failed')
      },
      undo: () => {
        if (undoShouldFail) throw new Error('undo failed')
      },
    }

    await history.execute(command)
    await expect(history.undo()).rejects.toThrow('undo failed')
    expect(history.canUndo.value).toBe(true)
    expect(history.undoLabel.value).toBe('Recoverable')
    expect(history.canRedo.value).toBe(false)

    undoShouldFail = false
    await history.undo()
    redoShouldFail = true
    await expect(history.redo()).rejects.toThrow('redo failed')
    expect(history.canUndo.value).toBe(false)
    expect(history.canRedo.value).toBe(true)
    expect(history.redoLabel.value).toBe('Recoverable')
  })

  it('clears both stacks in queue order', async () => {
    const history = useWorkflowHistory()
    await history.record({ label: 'Recorded', redo: () => undefined, undo: () => undefined })
    await history.undo()

    await history.clear()

    expect(history.canUndo.value).toBe(false)
    expect(history.canRedo.value).toBe(false)
    expect(history.undoLabel.value).toBe('')
    expect(history.redoLabel.value).toBe('')
  })
})
