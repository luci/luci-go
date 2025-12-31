# Async patterns

Utilities for managing asynchronous operations and optimization.

## batched

Wraps a function to automatically batch multiple calls into a single execution.

### Usage

```ts
import { batched } from '@/generic_libs/tools/batched_fn';

const fetchBuildsBatched = batched({
  fn: (ids: string[]) => fetchManyBuilds(ids),
  shardFn: (id) => 'default-shard', // Optional: group by shard
  combineParamSets: (ids1, ids2) => {
    // Logic to combine params, e.g., concat arrays
    if (ids1.length + ids2.length > 100) return { ok: false, error: 'too many' };
    return { ok: true, value: [...ids1, ...ids2] };
  },
  splitReturn: (paramSets, result) => {
    // Logic to split the result back to individual calls
    let offset = 0;
    return paramSets.map(set => {
      const slice = result.slice(offset, offset + set.length);
      offset += set.length;
      return slice;
    });
  }
});

// These will be combined into one fetch call
fetchBuildsBatched({}, '1');
fetchBuildsBatched({}, '2');
```

### Internal workings

It queues calls and waits for a microtask or timeout (`maxPendingMs`). When
triggered, it executes `combineParamSets` to merge arguments. If merging fails
(e.g., batch full), it executes the pending batch immediately and starts a new
one.

---

## cached

Wraps a function to cache its results based on parameters.

### Usage

```ts
import { cached } from '@/generic_libs/tools/cached_fn';

const expensiveCalc = cached(
  (a, b) => performCalculation(a, b),
  {
    key: (a, b) => `${a}-${b}`,
    expire: async (params, result) => {
        // Optional: wait for something then expire
        await new Promise(r => setTimeout(r, 5000));
    }
  }
);

expensiveCalc({}, 1, 2); // Computes
expensiveCalc({}, 1, 2); // Returns cached result
```

### Options

*   `acceptCache`: If false, ignores existing cache (like force refresh).
*   `skipUpdate`: If true, calculates but doesn't update cache.
*   `invalidateCache`: Removes the entry after returning.
