// Copyright 2021 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * @fileoverview
 * This file provides utilities to attach tags to an object. This can be very
 * useful when handling errors.
 *
 * For example:
 * ```typescript
 * async function fetchData() {
 *   const res = await fetch('/data-endpoint');
 *   if (res.status === 401) {
 *     const err = new Error('auth-error');
 *     attachTags(err, 'requires-signin');
 *     throw err;
 *   }
 *   ...
 * }
 *
 * async function main() {
 *   try {
 *     ...
 *     await fetchData();
 *     ...
 *   } catch (e) {
 *     if (hasTags(e, 'requires-signin')) {
 *       // Trigger sign in flow.
 *       ...
 *     }
 *   }
 * }
 * ```
 */

export const TAG_SOURCE = Symbol('TAG_SOURCE');

/**
 * An interface that allows objects to include the tags from inner properties
 * when performing hasTags test.
 *
 * An object (`target`) is considered to have the tag attached to it if
 *  1. the tag is attached to `target`, or
 *  2. the tag is attached to `target[TAG_SOURCE]`.
 *
 * This is useful when you want to wrap an error without losing the attached
 * tags.
 *
 * Example:
 * ```typescript
 * class WrapperError extends Error implements InnerTag {
 *   [TAG_SOURCE]: Error;
 *
 *   constructor(source: Error) {
 *     super();
 *     this[TAG_SOURCE] = source;
 *   }
 * }
 *
 * const source = new Error();
 * const wrapper = new WrapperError(source);
 * attachTags(source, 'tag');
 * assert.isTrue(hasTags(wrapper, 'tag'));
 * ```
 */
export interface InnerTag {
  readonly [TAG_SOURCE]: object;
}

// Use WeakMap instead of symbol property so
// 1. there's less type casting required when accessing tags, and
// 2. the implementation can work with frozen objects.
const tagMap = new WeakMap<object, Set<unknown>>();

/**
 * Attaches tags to the target.
 *
 * @returns target. Returns the target for easy chaining.
 */
export function attachTags<T extends object>(target: T, ...tags: unknown[]): T {
  let tagSet = tagMap.get(target);
  if (!tagSet) {
    tagSet = new Set();
    tagMap.set(target, tagSet);
  }
  for (const tag of tags) {
    tagSet.add(tag);
  }
  return target;
}

/**
 * Determines whether the target has the specified tags attached to it
 * (directly or recursively).
 *
 * Returns true if all tags are attached to the target.
 * Returns false otherwise.
 *
 * See the documentation for `InnerTag` for the recursive rule.
 */
export function hasTags(target: object | InnerTag, ...tags: unknown[]): boolean {
  const missingTags = tags.filter((tag) => !tagMap.get(target)?.has(tag));
  if (missingTags.length === 0) {
    return true;
  }

  if (Object.prototype.hasOwnProperty.call(target, TAG_SOURCE)) {
    return hasTags((target as InnerTag)[TAG_SOURCE], ...missingTags);
  }

  return false;
}
