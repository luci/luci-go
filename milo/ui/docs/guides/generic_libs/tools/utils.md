# Utility tools

General purpose utility functions.

## Common utilities
(From `src/generic_libs/tools/utils/utils.ts`)

*   **`deferred()`**: Returns `[promise, resolve, reject]` tuple. Useful for
    resolving promises outside the executor.
*   **`toError(unknown)`**: Safely converts unknown values to `Error` objects.
*   **`timeout(ms)`**: Promisified setTimeout.
*   **`URLExt`**: Extended `URL` class with `removeMatchedParams` to clean up
    default values.
*   **`getObjectId(obj)`**: Assigns and retrieves a stable sequential ID for an
    object reference (using WeakMap).
*   **`sha256(msg)`**: Async SHA-256 hashing using SubtleCrypto.

## String utilities
(From `src/generic_libs/tools/string_utils`)

*   **`countMatches`**: Checks how many characters match between two strings.
*   **`caseInsensitiveMatch`**: Checks if two strings match case-insensitively.

## Number utilities
(From `src/generic_libs/tools/num_utils`)

*   **`round`**: Rounds a number to specific precision.
*   **`Range`**: Type alias for `[start, end]`.
*   **`intersect(range1, range2)`**: Computes intersection of two ranges.
*   **`splitRange(range, interval)`**: Splits a range into chunks aligned with
    the interval.

## Iteration utilities
(From `src/generic_libs/tools/iter_utils.ts`)

*   **`seq(n)`**: Generates a sequence of numbers from 0 to n-1.
