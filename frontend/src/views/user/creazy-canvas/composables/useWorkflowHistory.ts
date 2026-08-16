import { computed, ref, shallowRef } from 'vue'

type Awaitable<T> = T | PromiseLike<T>

export interface WorkflowCommand {
  readonly label: string
  redo: () => Awaitable<void>
  undo: () => Awaitable<void>
}

interface QueuedOperation {
  operation: () => Awaitable<unknown>
  resolve: (value: unknown) => void
  reject: (reason?: unknown) => void
}

export const WORKFLOW_HISTORY_LIMIT = 100

export function useWorkflowHistory(maxEntries = WORKFLOW_HISTORY_LIMIT) {
  const limit = Number.isFinite(maxEntries)
    ? Math.min(WORKFLOW_HISTORY_LIMIT, Math.max(1, Math.floor(maxEntries)))
    : WORKFLOW_HISTORY_LIMIT

  const undoStack = shallowRef<WorkflowCommand[]>([])
  const redoStack = shallowRef<WorkflowCommand[]>([])
  const pendingCount = ref(0)
  const queue: QueuedOperation[] = []
  let processing = false

  const isBusy = computed(() => pendingCount.value > 0)
  const canUndo = computed(() => undoStack.value.length > 0 && !isBusy.value)
  const canRedo = computed(() => redoStack.value.length > 0 && !isBusy.value)
  const undoLabel = computed(() => undoStack.value[undoStack.value.length - 1]?.label ?? '')
  const redoLabel = computed(() => redoStack.value[redoStack.value.length - 1]?.label ?? '')

  function append(stack: WorkflowCommand[], command: WorkflowCommand) {
    const next = [...stack, command]
    return next.length > limit ? next.slice(next.length - limit) : next
  }

  function runNext() {
    if (processing) return

    const queued = queue.shift()
    if (!queued) return

    processing = true
    let result: Awaitable<unknown>

    try {
      result = queued.operation()
    } catch (error) {
      queued.reject(error)
      finishOperation()
      return
    }

    Promise.resolve(result).then(
      (value) => {
        finishOperation()
        queued.resolve(value)
      },
      (error) => {
        finishOperation()
        queued.reject(error)
      },
    )
  }

  function finishOperation() {
    pendingCount.value -= 1
    processing = false
    runNext()
  }

  function enqueue<T>(operation: () => Awaitable<T>): Promise<T> {
    pendingCount.value += 1

    return new Promise<T>((resolve, reject) => {
      queue.push({
        operation,
        resolve: (value) => resolve(value as T),
        reject,
      })
      runNext()
    })
  }

  function execute(command: WorkflowCommand): Promise<void> {
    return enqueue(async () => {
      await command.redo()
      undoStack.value = append(undoStack.value, command)
      redoStack.value = []
    })
  }

  function record(command: WorkflowCommand): Promise<void> {
    return enqueue(() => {
      undoStack.value = append(undoStack.value, command)
      redoStack.value = []
    })
  }

  function undo(): Promise<boolean> {
    return enqueue(async () => {
      const command = undoStack.value[undoStack.value.length - 1]
      if (!command) return false

      await command.undo()
      undoStack.value = undoStack.value.slice(0, -1)
      redoStack.value = append(redoStack.value, command)
      return true
    })
  }

  function redo(): Promise<boolean> {
    return enqueue(async () => {
      const command = redoStack.value[redoStack.value.length - 1]
      if (!command) return false

      await command.redo()
      redoStack.value = redoStack.value.slice(0, -1)
      undoStack.value = append(undoStack.value, command)
      return true
    })
  }

  function clear(): Promise<void> {
    return enqueue(() => {
      undoStack.value = []
      redoStack.value = []
    })
  }

  return {
    canUndo,
    canRedo,
    isBusy,
    undoLabel,
    redoLabel,
    execute,
    record,
    undo,
    redo,
    clear,
  }
}
