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

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';

import { getAndroidColumns } from './android_columns';
import { AndroidColumnDef } from './android_fields';

describe('Android Columns', () => {
  it('should generate proper order_by_fields for base dimensions mapped as labels', () => {
    const columns = getAndroidColumns(['model', 'id', 'state']);

    const modelCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'model',
    ) as AndroidColumnDef;
    const idCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'id',
    ) as AndroidColumnDef;

    expect(modelCol.orderByField).toBe('labels.model');
    expect(idCol.orderByField).toBe('id');
  });

  describe('normalizeFilterKey', () => {
    it('should unquote non-labels keys', () => {
      expect(normalizeFilterKey('"id"')).toBe('id');
    });

    it('should not unquote labels keys', () => {
      expect(normalizeFilterKey('labels.model')).toBe('model');
    });

    it('should unquote values passing through it', () => {
      expect(normalizeFilterKey('"IDLE"')).toBe('IDLE');
    });

    it('should leave plain values untouched', () => {
      expect(normalizeFilterKey('IDLE')).toBe('IDLE');
    });
  });
});
