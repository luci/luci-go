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

import { TimeInterval } from '@/clusters/hooks/use_fetch_clusters';

// The time interval options from which to select.
// Note: the ClustersTable component uses the first entry as the default
// selection.
export const TIME_INTERVAL_OPTIONS: TimeInterval[] = [
  {
    id: '24h',
    label: 'Last 24 hours',
    duration: 24,
  },
  {
    id: '3d',
    label: 'Last 3 days',
    duration: 3 * 24,
  },
  {
    id: '7d',
    label: 'Last 7 days',
    duration: 7 * 24,
  },
];
