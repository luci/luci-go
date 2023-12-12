// Copyright 2023 The LUCI Authors.
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

import { ORDERED_VARIANT_DEF_KEYS } from '@/common/constants/test';

/**
 * Parses the test result name and get the individual components.
 */
export function parseTestResultName(name: string) {
  const match = name.match(
    /^invocations\/(.*?)\/tests\/(.*?)\/results\/(.*?)$/,
  );
  if (!match) {
    throw new Error(`invalid test result name: ${name}`);
  }

  const [, invocationId, testId, resultId] = match;
  return {
    invocationId,
    testId: decodeURIComponent(testId),
    resultId,
  };
}

export function getSortedTestVariantDef(def: { [key: string]: string }) {
  const res: Array<[string, string]> = [];
  const seen = new Set();
  for (const key of ORDERED_VARIANT_DEF_KEYS) {
    if (Object.prototype.hasOwnProperty.call(def, key)) {
      res.push([key, def[key]]);
      seen.add(key);
    }
  }
  for (const [key, value] of Object.entries(def)) {
    if (!seen.has(key)) {
      res.push([key, value]);
    }
  }
  return res;
}
