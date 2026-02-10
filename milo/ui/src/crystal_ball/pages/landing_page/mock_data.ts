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

import { Dashboard } from './types';

export const mockDashboards: Dashboard[] = [
  {
    id: '1',
    name: 'Generic Dashboard Alpha',
    description: 'This is the first test dashboard.',
    lastModified: DateTime.now().minus({ days: 2 }).toISO()!,
  },
  {
    id: '2',
    name: 'Generic Dashboard Beta',
    description: 'This is the second test dashboard.',
    lastModified: DateTime.now().minus({ days: 5 }).toISO()!,
  },
  {
    id: '3',
    name: 'Test View Gamma',
    description: 'Additional details for the gamma view.',
    lastModified: DateTime.now().minus({ weeks: 1 }).toISO()!,
  },
  {
    id: '4',
    name: 'Sample Dashboard Delta',
    description: 'Information about the delta dashboard.',
    lastModified: DateTime.now().minus({ weeks: 2 }).toISO()!,
  },
  ...Array.from({ length: 20 }).map((_, i) => ({
    id: `mock-${i + 5}`,
    name: `Mock Dashboard ${i + 5}`,
    description: `Description for mock dashboard ${i + 5}.`,
    lastModified: DateTime.now()
      .minus({ days: i * 10 + 3 })
      .toISO()!,
  })),
];
