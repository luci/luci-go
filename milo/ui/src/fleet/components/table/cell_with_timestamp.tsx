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
import { MRT_RowData } from 'material-react-table';
import React from 'react';

import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { FC_CellProps } from '@/fleet/types/table';

export interface TimestampDisplayProps {
  value?: unknown;
}

/**
 * Common cell renderer helper for timestamp fields across Fleet Console tables.
 * Formats valid ISO timestamps using <SmartRelativeTimestamp /> and falls back cleanly for invalid or missing dates.
 */
export function renderTimestampCell<R extends MRT_RowData>(
  props: FC_CellProps<R> | TimestampDisplayProps,
): React.ReactNode {
  const value = 'cell' in props ? props.cell.getValue() : props.value;
  const rawValue = Array.isArray(value) ? value[0] : value;
  if (rawValue === null || rawValue === undefined || rawValue === '') {
    return '-';
  }
  const strValue = String(rawValue);
  try {
    const dt = DateTime.fromISO(strValue);
    if (!dt.isValid) {
      return <>{strValue}</>;
    }
    return <SmartRelativeTimestamp date={dt} />;
  } catch (_) {
    return <>{strValue}</>;
  }
}
