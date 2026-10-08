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

/**
 * Formats a signed point value (priority score or rule weight) for display,
 * e.g. `+350 pts`, `-50 pts`, `0 pts`. Values are kept as strings so int64
 * scores are not truncated.
 */
export const formatPoints = (value?: string | number): string => {
  const valueStr = typeof value === 'number' ? String(value) : value || '0';
  if (!valueStr || valueStr === '0' || valueStr === '-0') {
    return '0 pts';
  }
  if (valueStr.startsWith('-')) {
    const absVal = valueStr.slice(1);
    return `-${absVal} pts`;
  }
  const clean = valueStr.startsWith('+') ? valueStr.slice(1) : valueStr;
  return `+${clean} pts`;
};

export const getBugUrl = (bugId: string): string =>
  `https://b.corp.google.com/issues/${encodeURIComponent(bugId)}`;
