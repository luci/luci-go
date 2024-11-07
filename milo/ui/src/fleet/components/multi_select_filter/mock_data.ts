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

import { FilterOption } from './types';

export const FILTER_OPTIONS: FilterOption[] = [
  {
    label: 'Option 1',
    value: 'val-1',
    options: [
      { label: 'The first option', value: 'o11' },
      { label: 'The second option', value: 'o12' },
    ],
  },
  {
    label: 'Option 2',
    value: 'val-2',
    options: [
      { label: 'The first option', value: 'o21' },
      { label: 'The second option', value: 'o22' },
    ],
  },
  {
    label: 'Option 3',
    value: 'val-3',
    options: [
      { label: 'The first option', value: 'o31' },
      { label: 'The second option', value: 'o32' },
    ],
  },
  {
    label: 'Option 4',
    value: 'val-4',
    options: [
      { label: 'The first option', value: 'o41' },
      { label: 'The second option', value: 'o42' },
    ],
  },
];
