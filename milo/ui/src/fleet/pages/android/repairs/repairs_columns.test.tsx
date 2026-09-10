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

import { MRT_ColumnDef } from 'material-react-table';
import { isValidElement } from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { ORDER_BY_PARAM_KEY } from '@/fleet/hooks/order_by';
import { AndroidPageWorkspace } from '@/fleet/workspaces';

import { getRepairsColumns } from './repairs_columns';
import { Row } from './repairs_columns.utils';

describe('repairs_columns explore devices link', () => {
  const getExploreDevicesTo = (
    workspace: AndroidPageWorkspace,
    row: Partial<Row>,
  ): string => {
    const columns = getRepairsColumns(workspace);
    const exploreDevicesCol = columns['static-explore_devices'];
    const cellContext = {
      row: {
        original: {
          id: 'test-id',
          priority: 0,
          lab_name: '',
          host_group: '',
          run_target: '',
          minimum_repairs: 0,
          devices_offline_ratio: '0 / 0',
          devices_offline_percentage: '0%',
          peak_usage: 0,
          total_devices: 0,
          ...row,
        } as Row,
      },
    } as Parameters<NonNullable<MRT_ColumnDef<Row>['Cell']>>[0];

    const element = exploreDevicesCol.Cell?.(cellContext);
    if (!isValidElement<{ to: string }>(element)) {
      throw new Error('ExploreDevicesLink element not returned');
    }
    return element.props.to;
  };

  describe('works both for android and pixel', () => {
    it('generates correct explore devices link for Android workspace', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: 'lab1',
        host_group: 'group1',
        run_target: 'target1',
      });

      const url = new URL(to, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/android/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "lab1") AND ("host_group" = "group1") AND ("run_target" = "target1")',
      );
    });

    it('generates correct explore devices link for Pixel workspace', () => {
      const to = getExploreDevicesTo('Pixel', {
        lab_name: 'pixel_lab',
        host_group: 'custom_group',
        run_target: 'pixel_target',
      });

      const url = new URL(to, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/pixel/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "pixel_lab") AND ("host_group" = "custom_group") AND ("run_target" = "pixel_target")',
      );
    });

    it('filters out pte_labs host_group for Pixel workspace', () => {
      const to = getExploreDevicesTo('Pixel', {
        lab_name: 'pixel_lab',
        host_group: 'pte_labs',
        run_target: 'pixel_target',
      });

      const url = new URL(to, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/pixel/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "pixel_lab") AND ("run_target" = "pixel_target")',
      );
    });
  });

  describe('empty metric handling generates include blank in the url', () => {
    it('generates include blank for lab_name when empty string', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: '',
        host_group: 'group1',
        run_target: 'target1',
      });

      const url = new URL(to, 'http://localhost');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        'NOT "lab_name":* AND ("host_group" = "group1") AND ("run_target" = "target1")',
      );
      expect(filters).toContain('NOT "lab_name":*');
    });

    it('generates include blank for host_group when empty string', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: 'lab1',
        host_group: '',
        run_target: 'target1',
      });

      const url = new URL(to, 'http://localhost');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        '("lab_name" = "lab1") AND NOT "host_group":* AND ("run_target" = "target1")',
      );
      expect(filters).toContain('NOT "host_group":*');
    });

    it('generates include blank for run_target when empty string', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: 'lab1',
        host_group: 'group1',
        run_target: '',
      });

      const url = new URL(to, 'http://localhost');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        '("lab_name" = "lab1") AND ("host_group" = "group1") AND NOT "run_target":*',
      );
      expect(filters).toContain('NOT "run_target":*');
    });

    it('generates include blank for all metrics when lab_name, host_group, and run_target are all empty', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: '',
        host_group: '',
        run_target: '',
      });

      const url = new URL(to, 'http://localhost');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        'NOT "lab_name":* AND NOT "host_group":* AND NOT "run_target":*',
      );
      expect(filters).toContain('NOT "lab_name":*');
      expect(filters).toContain('NOT "host_group":*');
      expect(filters).toContain('NOT "run_target":*');
    });

    it('generates include blank for empty metrics in Pixel workspace', () => {
      const to = getExploreDevicesTo('Pixel', {
        lab_name: '',
        host_group: '',
        run_target: 'pixel_target',
      });

      const url = new URL(to, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/pixel/devices');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        'NOT "lab_name":* AND NOT "host_group":* AND ("run_target" = "pixel_target")',
      );
      expect(filters).toContain('NOT "lab_name":*');
      expect(filters).toContain('NOT "host_group":*');
    });

    it('handles BLANK_VALUE directly if provided as metric value', () => {
      const to = getExploreDevicesTo('Android', {
        lab_name: BLANK_VALUE,
        host_group: 'group1',
        run_target: 'target1',
      });

      const url = new URL(to, 'http://localhost');
      const filters = url.searchParams.get(FILTERS_PARAM_KEY);
      expect(filters).toBe(
        'NOT "lab_name":* AND ("host_group" = "group1") AND ("run_target" = "target1")',
      );
      expect(filters).toContain('NOT "lab_name":*');
    });
  });
});
