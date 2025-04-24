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

import { SummaryStyle } from '../constants';

export interface LogsTableEntry {
  entryId: string;
  logFile?: string;
  fileNumber?: number;
  timestamp?: string;
  severity?: string;
  summary: string;
  fullName?: string;
  line?: number;
  summaryStyle?: SummaryStyle;
  normalizedSummary?: string;
  // Whether to enable the context action menu.
  enableContextActionMenu?: boolean;
  // Comparison mode controls
  chunkId?: number;
  enableExpansion?: boolean;
}
