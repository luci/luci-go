// Copyright 2026 The LUCI Authors.
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

export const formatEnum = (
  val: number | string | null | undefined,
  toJSONFn: (v: number) => string,
  prefixToRemove = '',
): string => {
  if (val === null || val === undefined) return 'N/A';
  if (typeof val === 'number') {
    try {
      const raw = toJSONFn(val);
      if (raw) {
        if (prefixToRemove && raw.startsWith(prefixToRemove)) {
          return raw.slice(prefixToRemove.length);
        }
        return raw;
      }
    } catch {
      // Fallback if conversion fails
    }
  }
  const strVal = String(val);
  if (prefixToRemove && strVal.startsWith(prefixToRemove)) {
    return strVal.slice(prefixToRemove.length);
  }
  return strVal;
};

export const safeFormatDate = (
  dateStr: string | number | null | undefined,
): string => {
  if (!dateStr) return 'N/A';
  try {
    const d = new Date(dateStr);
    return isNaN(d.getTime()) ? String(dateStr) : d.toLocaleString();
  } catch {
    return String(dateStr);
  }
};
