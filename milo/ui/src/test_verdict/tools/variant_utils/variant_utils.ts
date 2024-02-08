// Copyright 2024 The LUCI Authors.
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

import { groupBy } from 'lodash-es';

import { Variant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

/**
 * A list of variant keys that have special meanings. Ordered by their
 * significance.
 */
const SPECIAL_VARIANT_KEYS = ['builder', 'test_suite'];

/**
 * Returns the order of the variant key.
 */
function getVariantKeyOrder(key: string) {
  const i = SPECIAL_VARIANT_KEYS.indexOf(key);
  if (i === -1) {
    return SPECIAL_VARIANT_KEYS.length;
  }
  return i;
}

/**
 * Given a list of variants, return a list of variant keys that can uniquely
 * identify each unique variant in the list.
 */
export function getCriticalVariantKeys(variants: readonly Variant[]): string[] {
  // Get all the keys.
  const keys = new Set<string>();
  for (const variant of variants) {
    for (const key of Object.keys(variant.def)) {
      keys.add(key);
    }
  }

  // Sort the variant keys. We prefer keys in SPECIAL_VARIANT_KEYS over others.
  const orderedKeys = [...keys.values()].sort((key1, key2) => {
    const priorityDiff = getVariantKeyOrder(key1) - getVariantKeyOrder(key2);
    if (priorityDiff !== 0) {
      return priorityDiff;
    }
    return key1.localeCompare(key2);
  });

  // Find all the critical keys.
  const criticalKeys: string[] = [];
  let variantGroups = [variants];
  for (const key of orderedKeys) {
    const newGroups: typeof variantGroups = [];
    for (const group of variantGroups) {
      newGroups.push(...Object.values(groupBy(group, (g) => g.def[key])));
    }

    // Group by this key split the groups into more groups. Add this key to the
    // critical key list.
    if (newGroups.length !== variantGroups.length) {
      criticalKeys.push(key);
      variantGroups = newGroups;
    }
  }

  // Add at least one key to the critical key list.
  if (criticalKeys.length === 0 && orderedKeys.length !== 0) {
    criticalKeys.push(orderedKeys[0]);
  }

  return criticalKeys;
}
