# State Management Hooks

Hooks for managing complex state scenarios.

## useSyncedSearchParams

A hook similar to `react-router`'s `useSearchParams`, but with critical
improvements for concurrent updates.

### Problem solved
Native `useSearchParams` does not merge updates if called multiple times in the
same render cycle or in quick succession without waiting for the router update.
This leads to race conditions where one update overwrites another.

`useSyncedSearchParams` solves this by providing a "reducer-like" update
mechanism where updates are applied based on the *latest* pending state, not the
committed state.

### Usage

```tsx
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

function MyComponent() {
  const [params, setParams] = useSyncedSearchParams();

  const updateFilters = () => {
    // These two updates will effectively MERGE, unlike native useSearchParams
    setParams(prev => {
      const newParams = new URLSearchParams(prev);
      newParams.set('filterA', 'value');
      return newParams;
    });

    setParams(prev => {
      const newParams = new URLSearchParams(prev);
      newParams.set('filterB', 'value');
      return newParams;
    });
  };

  return <button onClick={updateFilters}>Update</button>;
}
```

### Requirements

Must be used within a `SyncedSearchParamsProvider` (usually at the root of your
application/route).

---

### Internal workings

It maintains an internal queue of updates and applies them sequentially to the
`URLSearchParams` object. It ensures that the `setParams` callback always
receives the state including *pending* updates, avoiding the "lost update"
problem common in async state synchronizers.
