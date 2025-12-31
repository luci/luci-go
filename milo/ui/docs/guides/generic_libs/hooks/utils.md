# Utility Hooks

General purpose hooks.

## useSingleton

Enables a singleton pattern specifically scoped to a React Context provider,
rather than global module scope.

### Usage

```tsx
import { useSingleton } from '@/generic_libs/hooks/singleton';

function MyComponent() {
  // 'my-service' instance will be created ONCE per SingletonStoreProvider
  // and shared among all components requesting this key.
  const service = useSingleton({
    key: ['my-service', configId],
    fn: () => new MyService(configId),
  });

  return <div>{service.data}</div>;
}
```

### Features

*   **Scoped**: Singletons live as long as the `SingletonStoreProvider` lives
    (or until no longer used).
*   **Reference Counting**: The instance is cached only while there is at least
    one active component using it. When the last consumer unmounts, the instance
    is cleaned up (if the instance supports cleanup/disposal, check
    implementation details).
*   **Stable Identity**: Returns the exact same object reference to all consumers.

### Internal workings

Uses a `SingletonStore` in context which maps stable stringified keys to
instances. It implements a reference counting mechanism using
`subscribe`/`unsubscribe` called from `useEffect`.

---

## useIsDevBuild

Returns `true` if the application is running in a local development environment.

### Usage

```tsx
import { useIsDevBuild } from '@/generic_libs/hooks/is_dev_build';

function MyDebugComponent() {
  const isDev = useIsDevBuild();

  if (!isDev) return null;

  return <DebugTools />;
}
```

### Note
This distinguishes *local development* (localhost) from *deployed builds*. It
typically does NOT return true for staging environments deployed to the cloud.
