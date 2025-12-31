# Data Fetching Hooks

Hooks for advanced data fetching scenarios.

## useVirtualizedQuery

A powerful hook for efficiently fetching large lists of data in chunks, designed
to work with virtualized lists (like `react-window` or `react-virtuoso`).

### Problem solved
When rendering a list with thousands of items, you only want to fetch the items
currently visible (plus a buffer). Standard `useQuery` fetches everything or
requires manual pagination management. `useVirtualizedQuery` automates this by
splitting the total range into "chunks" and only fetching/caching chunks that
overlap with the user's current view.

### Usage

```tsx
import { useVirtualizedQuery } from '@/generic_libs/hooks/virtualized_query';
import { useQuery } from '@tanstack/react-query';

function MyList({ start, end }) {
  const query = useVirtualizedQuery({
    // The total known range (e.g., 0 to 10000)
    rangeBoundary: [0, 10000],

    // How large each chunk should be (e.g., 50 items per network request)
    interval: 50,

    // Initial range to fetch
    initRange: [0, 50],

    // Function to generate a react-query options object for a specific chunk
    // [start, end)
    genQuery: (chunkStart, chunkEnd) => ({
      queryKey: ['my-list', chunkStart, chunkEnd],
      queryFn: () => fetchItems(chunkStart, chunkEnd),
      enabled: true,
    }),
  });

  // Update the requested range when the user scrolls
  useEffect(() => {
    query.setRange([start, end]);
  }, [start, end]);

  // Retrieve a specific item
  const itemResult = query.get(someIndex);

  if (itemResult.isPending) return <Skeleton />;
  return <Item data={itemResult.data} />;
}
```

### Internal workings

1.  **Chunking**: Splits the global `rangeBoundary` into aligned chunks based on
    `interval`.
2.  **Intersection**: Calculates which chunks intersect with the requested
    `setRange`.
3.  **UseQueries**: Uses `useQueries` (from `@tanstack/react-query`) to fetch
    multiple chunks in parallel.
4.  **Reference Counting** (Implicit): By deriving `enabled` status from the
    current `settledRange` (with optional delay), it avoids fetching chunks if
    the user scrolls past them quickly.

---

### Parameters

*   `rangeBoundary`: `[start, end]` - The absolute limits of data.
*   `interval`: Size of each chunk.
*   `genQuery`: Factory function returning query options for a chunk.
*   `delayMs`: (Optional) Debounce time. Queries for new chunks aren't fired
    until the viewport stays on them for this duration.
