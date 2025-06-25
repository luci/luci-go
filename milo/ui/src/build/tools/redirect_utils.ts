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

/**
 * A configuration object that defines each property key and its logical group.
 * Using "as const" allows TypeScript to infer the most specific types.
 */
const PROP_KEY_CONFIG = {
  'ID:': { group: 'testId' },
  'ExactID:': { group: 'testId' },
  'VHash:': { group: 'vhash' },
  'V:': { group: 'variantDef' },
} as const;

/**
 * A TypeScript type representing one of the valid property keys.
 * e.g., 'ID:' | 'ExactID:' | 'VHash:' | 'V:'
 */
type PropKey = keyof typeof PROP_KEY_CONFIG;

/**
 * A runtime array containing all valid property keys, derived from the config.
 */
const ALL_KEYS = Object.keys(PROP_KEY_CONFIG) as PropKey[];

/**
 * Gets the logical group for a given property key using the configuration object.
 */
export function getPropertyGroup(key: string): string {
  if (key in PROP_KEY_CONFIG) {
    return PROP_KEY_CONFIG[key as PropKey].group;
  }
  return 'unknown';
}

export function parseDeepLinkQuery(q: string | null): {
  testId: string | null;
  variantHash: string | null;
  variantDef: Record<string, string> | null;
} {
  if (!q) {
    return { testId: null, variantHash: null, variantDef: null };
  }

  let testId: string | null = null;
  let variantHash: string | null = null;
  const variantDef: Record<string, string> = {};

  const foundKeys: { key: PropKey; index: number }[] = [];
  for (const key of ALL_KEYS) {
    let startIndex = 0;
    while (startIndex < q.length) {
      const index = q.indexOf(key, startIndex);
      if (index === -1) {
        break;
      }
      foundKeys.push({ key, index });
      startIndex = index + key.length;
    }
  }

  foundKeys.sort((a, b) => a.index - b.index);

  const effectiveBoundaries: { key: PropKey; index: number }[] = [];
  if (foundKeys.length > 0) {
    effectiveBoundaries.push(foundKeys[0]);
    for (let i = 1; i < foundKeys.length; i++) {
      const current = foundKeys[i];
      const previous = effectiveBoundaries[effectiveBoundaries.length - 1];
      if (getPropertyGroup(current.key) === getPropertyGroup(previous.key)) {
        continue;
      }
      effectiveBoundaries.push(current);
    }
  }

  const processedGroups = new Set<string>();

  for (let i = 0; i < effectiveBoundaries.length; i++) {
    const currentBoundary = effectiveBoundaries[i];
    const nextBoundary = effectiveBoundaries[i + 1];
    const group = getPropertyGroup(currentBoundary.key);

    if (processedGroups.has(group)) {
      continue;
    }

    const valueStartIndex = currentBoundary.index + currentBoundary.key.length;
    const valueEndIndex = nextBoundary ? nextBoundary.index : q.length;

    const value = q.substring(valueStartIndex, valueEndIndex).trim();

    switch (group) {
      case 'testId':
        testId = decodeURIComponent(value);
        break;
      case 'vhash':
        variantHash = decodeURIComponent(value);
        break;
      case 'variantDef': {
        const firstEqIndex = value.indexOf('=');
        if (firstEqIndex > 0) {
          const vKey = decodeURIComponent(value.substring(0, firstEqIndex));
          const vValue = decodeURIComponent(value.substring(firstEqIndex + 1));
          variantDef[vKey] = vValue;
        }
        break;
      }
    }

    processedGroups.add(group);
  }

  const hasVariantDef = Object.keys(variantDef).length > 0;

  return {
    testId,
    variantHash,
    variantDef: hasVariantDef ? variantDef : null,
  };
}
