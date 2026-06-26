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

import { DateTime } from 'luxon';
import { ReactNode } from 'react';

import { Timestamp } from '@/common/components/timestamp';

/**
 * Parses an ISO timestamp string into a Luxon DateTime object.
 * Returns undefined if the string is empty or invalid.
 */
export function parseTimestamp(ts?: string): DateTime | undefined {
  if (!ts) {
    return undefined;
  }
  const dt = DateTime.fromISO(ts);
  return dt.isValid ? dt : undefined;
}

/**
 * Renders a standard LUCI Timestamp component for a given ISO string.
 * Returns undefined if the timestamp is invalid or missing.
 */
export function renderTimestamp(ts?: string): ReactNode {
  const dt = parseTimestamp(ts);
  return dt ? <Timestamp datetime={dt} /> : undefined;
}
