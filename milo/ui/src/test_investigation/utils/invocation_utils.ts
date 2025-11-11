// Copyright 2025 The LUCI Authors.
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

import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';

/**
 * A union type representing either a legacy Invocation or a new RootInvocation.
 */
export type AnyInvocation = Invocation | RootInvocation;

/**
 * Type guard to check if an object is a RootInvocation.
 * This is null/undefined-safe.
 */
export function isRootInvocation(
  inv: AnyInvocation | null | undefined,
): inv is RootInvocation {
  // `rootInvocationId` only exists on the RootInvocation proto.
  return !!inv && (inv as RootInvocation).rootInvocationId !== undefined;
}

/**
 * Type guard to check if an object is a legacy Invocation.
 * This is null/undefined-safe.
 */
export function isLegacyInvocation(
  inv: AnyInvocation | null | undefined,
): inv is Invocation {
  // An invocation is a legacy invocation if it's not null/undefined
  // and it's NOT a root invocation.
  return !!inv && !isRootInvocation(inv);
}

/**
 * Type guard to check if an object (which may be undefined) is a legacy Invocation.
 */
export function isLegacyInvocationData(
  inv: AnyInvocation | undefined,
): inv is Invocation {
  return !!inv && isLegacyInvocation(inv);
}

/**
 * Type guard to check if an object (which may be undefined) is a RootInvocation.
 */
export function isRootInvocationData(
  inv: AnyInvocation | undefined,
): inv is RootInvocation {
  return !!inv && isRootInvocation(inv);
}

/**
 * Generate the display invocation id.
 */
export function getDisplayInvocationId(invocation: AnyInvocation) {
  return invocation.name.substring(invocation.name.indexOf('/') + 1);
}
