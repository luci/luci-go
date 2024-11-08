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

import {
  Variant,
  Variant_DefEntry,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

// variantAsPairs converts a variant (mapping from keys
// to values) into a set of key-value pairs. key-value
// pairs are returned in sorted key order.
export function variantAsPairs(v?: Variant): Variant_DefEntry[] {
  const result: Variant_DefEntry[] = [];
  if (v === undefined) {
    return result;
  }
  for (const key of Object.keys(v.def)) {
    const value = v.def[key] || '';
    result.push({ key: key, value: value });
  }
  result.sort((a, b) => {
    return a.key.localeCompare(b.key, 'en');
  });
  return result;
}

// unionVariant returns a partial variant which contains
// only the key-value pairs shared by both v1 and v2.
// While it intersects the key-value pairs in both v1 and v2,
// the result is a union in the sense that filtering on the
// partial variant yields a set of varaints that includes
// both v1 and v2.
export function unionVariant(v1?: Variant, v2?: Variant): Variant {
  const result: Variant = { def: {} };
  if (v1 === undefined || v2 === undefined) {
    return result;
  }
  for (const key of Object.keys(v1.def)) {
    // Only keep keys that are both in v1 and v2.
    if (!Object.prototype.hasOwnProperty.call(v2.def, key)) {
      continue;
    }
    // Where the value at the key is the same in both v1 and v2.
    const value1 = v1.def[key] || '';
    const value2 = v2.def[key] || '';
    if (value1 === value2) {
      result.def[key] = value1;
    }
  }
  return result;
}
