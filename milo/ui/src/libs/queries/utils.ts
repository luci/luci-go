// Copyright 2022 The LUCI Authors.
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

const KV_RE = /([^=]*)(?:=(.*))?$/;

export const KV_SYNTAX_EXPLANATION =
  "case sensitive, both key & value should be URL query encoded if they contain '%', ' ', '+'";

/**
 * Parses a string like `${encodedKey}=${encodedValue}` and return the key and
 * the value. Both key and value in the string are assumed to be query param
 * encoded.
 *
 * If there's no value, (e.g. `${encodedKey}`), return null for the value.
 */
export function parseKeyValue(source: string): [string, string | null] {
  const [, key, value] = source.match(KV_RE)!;
  return [
    decodeQueryParam(key),
    value === undefined ? null : decodeQueryParam(value),
  ];
}

function decodeQueryParam(p: string) {
  return decodeURIComponent(p.replace(/\+/g, ' '));
}
