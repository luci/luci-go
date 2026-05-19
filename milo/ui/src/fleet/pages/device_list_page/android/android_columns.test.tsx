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

import React from 'react';

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

  it('should generate proper order_by_fields for average utilization columns', () => {
    const columns = getAndroidColumns(['average_7d', 'average_30d']);

    const avg7dCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'average_7d',
    ) as AndroidColumnDef;
    const avg30dCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'average_30d',
    ) as AndroidColumnDef;

    expect(avg7dCol.orderByField).toBe('average_7d');
    expect(avg30dCol.orderByField).toBe('average_30d');
  });

  it('should render null (empty cell) when average utilization is not a number, and proper percentage otherwise', () => {
    const columns = getAndroidColumns(['average_7d', 'average_30d']);

    const avg7dCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'average_7d',
    ) as AndroidColumnDef;
    const avg30dCol = columns.find(
      (c) => (c.accessorKey || c.id) === 'average_30d',
    ) as AndroidColumnDef;

    const mockCellParams7dZero = {
      cell: { getValue: () => 0 },
      row: { original: { average7d: 0 } },
    } as unknown as Parameters<Required<AndroidColumnDef>['Cell']>[0];

    const mockCellParams7dValue = {
      cell: { getValue: () => 42.5 },
      row: { original: { average7d: 42.5 } },
    } as unknown as Parameters<Required<AndroidColumnDef>['Cell']>[0];

    const mockCellParams7dNull = {
      cell: { getValue: () => null },
      row: { original: { average7d: null } },
    } as unknown as Parameters<Required<AndroidColumnDef>['Cell']>[0];

    const mockCellParams30dZero = {
      cell: { getValue: () => 0 },
      row: { original: { average30d: 0 } },
    } as unknown as Parameters<Required<AndroidColumnDef>['Cell']>[0];

    const mockCellParams30dValue = {
      cell: { getValue: () => 85.123 },
      row: { original: { average30d: 85.123 } },
    } as unknown as Parameters<Required<AndroidColumnDef>['Cell']>[0];

    expect(avg7dCol.Cell!(mockCellParams7dZero)).toEqual(
      <React.Fragment>{'0.00'}%</React.Fragment>,
    );
    expect(avg7dCol.Cell!(mockCellParams7dNull)).toBeNull();
    expect(avg7dCol.Cell!(mockCellParams7dValue)).toEqual(
      <React.Fragment>{'42.50'}%</React.Fragment>,
    );

    expect(avg30dCol.Cell!(mockCellParams30dZero)).toEqual(
      <React.Fragment>{'0.00'}%</React.Fragment>,
    );
    expect(avg30dCol.Cell!(mockCellParams30dValue)).toEqual(
      <React.Fragment>{'85.12'}%</React.Fragment>,
    );
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
