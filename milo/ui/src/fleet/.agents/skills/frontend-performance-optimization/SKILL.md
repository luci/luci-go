---
name: frontend-performance-optimization
description: Standard procedure, metrics guidelines, and referential stability standards for measuring and vetting frontend performance optimizations in the LUCI Fleet Console.
---

# Vetting & Measuring Frontend Performance Improvements

This skill defines the mandatory guidelines and best practices for identifying, implementing, profiling, and verifying frontend performance improvements in the LUCI Fleet Console (React/TypeScript) codebase.

---

## 1. Core Principles of Senior Frontend Performance Engineering

When engineering performance optimizations, we must prioritize concrete data and measurements over intuition. Every performance CL must be backed by reproducible profiling metrics.

### Key Performance Targets (SLA)

1. **Interactive Render Budget**: Column sorting, row selections, and tab transitions must finish rendering within **16ms** (1 frame) for simple states, and under **300ms** for heavy data grids (under simulated CPU throttling).
2. **DOM Element Footprint**: The active DOM node count must remain **under 2,500 elements**, even when displaying datasets exceeding 10,000 devices. This requires virtualization (`react-virtual` or `react-virtuoso`).
3. **Network Request Efficiency**: Column sorting and client-side filtering operations must trigger **0 new network requests** unless fetching a new page token.
4. **Referential Integrity**: Custom hooks must maintain strict referential stability. Re-rendering a page without state change must result in **zero reference changes** to inputs or hooks outputs.

---

## 2. Guidelines for Measuring UI Latency & Rendering

### Method A: High-Resolution User Timing API (Browser Execution)

To get exact render cycles during interaction, instrument code blocks using the standard Browser User Timing API. Note that we must use a nested "double `requestAnimationFrame`" pattern. The first `requestAnimationFrame` schedules a callback before the current frame's paint, and scheduling a second one inside it ensures the execution occurs right after the paint of that frame completes.

```typescript
// Generate a unique token per interaction to prevent concurrency race conditions
const interactionId =
  crypto.randomUUID?.() || Math.random().toString(36).slice(2);
const startMark = `interaction-start-${interactionId}`;
const paintMark = `interaction-paint-${interactionId}`;
const measureName = `Interaction Latency-${interactionId}`;

// 1. Mark the start of the user interaction
performance.mark(startMark);

// 2. Perform the state update
setFilterValue(newValue);

// 3. Measure in a double requestAnimationFrame to capture post-paint completion
requestAnimationFrame(() => {
  requestAnimationFrame(() => {
    performance.mark(paintMark);
    performance.measure(measureName, startMark, paintMark);

    const entries = performance.getEntriesByName(measureName);
    const measure = entries[entries.length - 1];
    console.log(`Render and Paint took: ${measure.duration.toFixed(2)}ms`);

    // Clean up specifically targeted entries for this interaction
    performance.clearMarks(startMark);
    performance.clearMarks(paintMark);
    performance.clearMeasures(measureName);
  });
});
```

> [!WARNING]
> **React Batching & Concurrent Rendering Limitation:** Because React state updates are asynchronous and heavily batched (especially in React 18+), imperatively scheduling a browser animation frame immediately after calling a state setter does not guarantee that the frame lines up precisely with React's commit and paint phases. Under Concurrent Rendering, the actual rendering work could be delayed or split. While this double `requestAnimationFrame` pattern is highly effective for measuring overall browser layout/paint cycles, the most accurate way to profile React-specific rendering is to use React's native `<Profiler>` API or measure durations inside synchronized hooks like `useLayoutEffect` and `useEffect`.

### Method B: Automated Hook Profiling (Unit & Integration Tests)

When writing tests for custom hooks, verify that re-rendering with identical inputs does not produce new object references.

```typescript
it("should maintain referential stability of return values if data does not change", () => {
  const stableInputs = { category: "model" };
  const { result, rerender } = renderHook(
    ({ hookInputs }) => useMyHook(hookInputs),
    { initialProps: { hookInputs: stableInputs } },
  );

  const initialResult = result.current;

  // Trigger a component re-render with identical input references
  rerender({ hookInputs: stableInputs });

  // Assert that reference equality (Object.is) is maintained
  expect(result.current).toBe(initialResult);
});
```

---

## 3. The "Silent Killer": Referential Instability in Custom Hooks

A common performance bottleneck in React applications is hook output instability. When a hook returns a new object reference on every render, it invalidates `useMemo` hooks, `React.memo` components, and React Query dependencies down the tree.

### ❌ The Anti-Pattern: Raw Object Return

Returning a new object literal triggers downstream re-renders of the entire table grid, even if the internal values are identical.

```typescript
// BAD: Recreates the wrapper object and functions on every single render
export const useChromeOSFilters = () => {
  const { filterValues, aip160 } = useFilters();

  return {
    filterValues,
    aip160,
    isLoading: false,
  };
};
```

### ✅ The Solution: Custom Hooks Output Stability

#### Option A: Simple hook output memoization

For standard custom hooks, wrap the returned object or array in `useMemo` to prevent recreation on every render cycle.

```typescript
// GOOD: Wrapper object is only recreated when internal state values change
export const useChromeOSFilters = () => {
  const { filterValues, aip160 } = useFilters();

  return useMemo(
    () => ({
      filterValues,
      aip160,
      isLoading: false,
    }),
    [filterValues, aip160],
  );
};
```

#### Option B: React Query Custom Hooks Downstream Memoization

When utilizing TanStack Query (`useQueries` / `useQuery`), note that the `combine` option does **NOT** structurally share or memoize its output object. Returning an object literal or map from `combine` will recreate a new reference on every single render cycle.

To ensure referential stability of your hook's output, capture the raw query results array and perform your data aggregation inside a downstream `useMemo` block. Because TanStack Query performs automatic structural sharing on the query data payloads inside the cache, the elements of the `results` array are referentially stable if the underlying data has not changed.

```typescript
// GOOD: Output reference is stable unless the aggregated data changes
export const useChromeOSCurrentTasks = (devices: Device[]) => {
  const dutIds = useMemo(() => devices.map(extractDutId), [devices]);
  const dutIdChunks = useMemo(() => chunkArray(dutIds, 100), [dutIds]);

  const queriesConfig = useMemo(() => {
    return dutIdChunks.map((chunk) => ({
      queryKey: ["tasks", chunk],
      queryFn: () => fetchTasks(chunk),
    }));
  }, [dutIdChunks]);

  const results = useQueries({
    queries: queriesConfig,
  });

  return useMemo(() => {
    const isPending = results.some((r) => r.isPending);
    const isError = results.some((r) => r.isError);
    const error = results.find((r) => r.error)?.error || null;

    const tasks: Record<string, TaskResult> = {};
    if (!isPending && !isError) {
      for (const query of results) {
        if (query.data) {
          Object.assign(tasks, query.data);
        }
      }
    }
    return { tasks, error, isPending, isError };
  }, [results]);
};
```

---

## 4. Performance Verification Checklist

Every frontend performance CL must detail the following metrics in its description:

1. **Baseline Metrics**:
   - Total DOM Node Count: `document.querySelectorAll('*').length`
   - Render Latency (Sorting/Filtering): Measured in milliseconds using marks.
   - Initial Bundle Size Impact: Measured in kB.
2. **Optimized Metrics**:
   - Show the reduction in render latency (e.g., from 400ms to <50ms).
   - Document any reduction in unnecessary network flurries.
3. **Scenario Stress Check**:
   - Verify performance under "CPU Throttling: 4x slowdown" in Chrome DevTools.
   - Confirm interaction responsiveness with 500+ rows visible.
4. **Post-Deployment Telemetry Verification Plan**:
   - Monitor `filter_changed` Google Analytics event latency values.
   - Verify `ListDevices` RPC invocation counts in logs.
