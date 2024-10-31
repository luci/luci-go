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

import { SummaryStyle } from '../constants/table_constants';
import { LogsTableEntry } from '../types';

export const createMockLogTableEntriesForScreenRecorder = (
  mockLogsPath: string,
): LogsTableEntry[] => {
  return Array(20)
    .fill(0)
    .map((_, idx) => {
      return {
        enableContextActionMenu: true,
        entryId: `${mockLogsPath}/dir1/log1:${idx}`,
        fileNumber: 1,
        fullName: `${mockLogsPath}/dir1/log${idx}`,
        line: idx,
        logFile: 'dir1/log1',
        normalizedSummary: '',
        summary: `Test info ... ${idx}`,
        timestamp: `2023-10-01T11:22:33.${String(idx).padStart(3, '0')}Z`,
        severity: 'INFO',
        summaryStyle: SummaryStyle.DEFAULT,
      };
    });
};
