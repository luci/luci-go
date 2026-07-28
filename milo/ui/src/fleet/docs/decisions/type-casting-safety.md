# Type Casting Safety Guidelines

## Context
Force-casting types (`as unknown as MyType`) bypasses TypeScript compiler checks and can hide runtime errors.

## Rule
Prefer automatic type narrowing (`typeof`, `instanceof`, or type guards). If you must use `as unknown as MyType` and the cast is not immediately obvious from surrounding context, add a `/// SAFETY:` comment explaining why the cast is safe.

### When Safety Comments Are Optional
A comment is optional if clear runtime verification precedes the cast:

```typescript
if (payload.type === 'user') {
  // Obvious narrowing check precedes cast
  const user = payload.data as unknown as UserProfile;
}
```

### When Safety Comments Are Required
Add a comment if correctness depends on backend contracts, external library guarantees, or invariants outside the immediate lines:

```typescript
/// SAFETY: Table configuration schema guarantees column types match MyExpectedType.
const columns = config.columns as unknown as MyExpectedType;
```
