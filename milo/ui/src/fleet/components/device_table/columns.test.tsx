// Copyright 2025 The LUCI Authors.
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

import { getColumns } from '@/fleet/pages/device_list_page/chromeos/columns';

import { orderColumns } from './columns';

describe('getColumns - Ordering Tests', () => {
  it('should order visible columns then the rest, each group alphabetically, prioritizing common ones', () => {
    const columnIds = [
      'type',
      'id', // common
      'dut_id', // common
      'port',
      'host',
      'state', // common
    ];
    const visibleColumnIds = ['type', 'dut_id', 'port'];

    const result = orderColumns(getColumns(columnIds), visibleColumnIds).map(
      (col) => col.field,
    ); // Extract just the field for ordering check

    expect(result).toEqual(['dut_id', 'port', 'type', 'id', 'state', 'host']);
  });

  it('ignore any non existing column from the visible columns', () => {
    const columnIds = ['col2', 'col1'];
    const visibleColumnIds = ['col3', 'col1'];

    const result = orderColumns(getColumns(columnIds), visibleColumnIds).map(
      (col) => col.field,
    );

    expect(result).toEqual(['col1', 'col2']);
  });
});
