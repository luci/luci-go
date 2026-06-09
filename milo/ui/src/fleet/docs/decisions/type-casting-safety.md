# Decision: Safety Comments for Type Casting (`as unknown as MyType`)

## Context

In TypeScript, force-casting a type using double casting (e.g., `as unknown as MyType`) bypasses the compiler's safety checks. While this is sometimes necessary when working with complex types, external libraries, or dynamic data, it can easily hide runtime type mismatches and lead to hard-to-debug crashes.

To ensure long-term maintainability and safety, we must document the reasoning behind non-obvious type assertions.

## Decision

Every time a type cast in the form of `as unknown as MyType` is used, and it is **not** immediately obvious from the context that this casting is always going to be correct, you **MUST** add a comment explaining why the type cast is safe.

### When is it "immediately obvious"?
A safety comment is **not** required if there is clear, immediate runtime verification right before the cast.

> [!NOTE]
> Most of the time, the TypeScript type checker can see type narrowing (e.g., via `instanceof`, `typeof`, or type guards) and automatically narrow the type. In these cases, you should prefer automatic narrowing and avoid explicit type casting altogether.

Example of an obvious cast when TypeScript cannot narrow automatically:
```typescript
// Assume this interface comes from an external library and cannot be changed.
interface ResponsePayload {
  type: 'user' | 'device';
  data: unknown;
}

if (payload.type === 'user') {
  // Immediately obvious that data is UserProfile because of the preceding check,
  // but the type definition for data is too generic for TypeScript to narrow.
  const user = payload.data as unknown as UserProfile;
}
```

### When is a `SAFETY` comment required?
A safety comment is **required** if the correctness of the cast depends on invariants, backend API guarantees, external library contracts, or logic elsewhere in the file that cannot be immediately seen by looking at the line itself.

The comment must use the following format:
```typescript
/// SAFETY: [reason]
```

### Examples

#### Correct Usage (with safety comment)
```typescript
// We receive data from an external library that is typed as `unknown`.
// However, the configuration guarantees that it matches `MyExpectedType`.
/// SAFETY: the table configuration only permits column definitions that align with MyExpectedType schema.
const columns = config.columns as unknown as MyExpectedType;
```

#### Incorrect Usage (missing safety comment)
```typescript
// AVOID: Bypassing compiler checks without explaining why this is safe
const data = response.payload as unknown as UserProfile;
```
