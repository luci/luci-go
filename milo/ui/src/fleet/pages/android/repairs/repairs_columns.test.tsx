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

import { getRepairsColumns, DEFAULT_COLUMNS } from './repairs_columns';

describe('Repairs Columns', () => {
  it('should return all default columns when passed DEFAULT_COLUMNS', () => {
    const columns = getRepairsColumns(DEFAULT_COLUMNS);
    expect(columns.length).toBe(DEFAULT_COLUMNS.length);
    expect(columns.map((c) => c.accessorKey)).toEqual(DEFAULT_COLUMNS);
  });

  it('should return a specific column when passed its ID', () => {
    const columns = getRepairsColumns(['priority']);
    expect(columns.length).toBe(1);
    expect(columns[0].accessorKey).toBe('priority');
  });

  it('should return an empty array when passed an invalid ID', () => {
    const columns = getRepairsColumns(['invalid_id']);
    expect(columns.length).toBe(0);
  });
});
