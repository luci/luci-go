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

import { renderHook } from '@testing-library/react';

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useIrmIncidentFilterBuilders } from './use_irm_incident_filter_builders';
import * as UseIrmIncidentsModule from './use_irm_incidents';
import {
  ACTIVE_IRM_FILTER_KEY,
  MODEL_HEALTH_FILTER_KEY,
  POOL_HEALTH_FILTER_KEY,
  RepairQueueFilterBuilder,
  useRepairQueueFilterBuilders,
} from './use_repair_queue_filter_builders';

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <FakeContextProvider>{children}</FakeContextProvider>
);

const renderFilterBuilders = () =>
  renderHook(() => useRepairQueueFilterBuilders(), { wrapper });

const mockDimensions = (data: GetDeviceDimensionsResponse) => {
  jest.spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions').mockReturnValue({
    data,
    isLoading: false,
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<
    typeof UseDeviceDimensionsModule.useDeviceDimensions
  >);
};

const mockIrmIncidents = (
  overrides: Partial<
    ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>
  > = {},
) => {
  jest.spyOn(UseIrmIncidentsModule, 'useIrmIncidents').mockReturnValue({
    data: { irmIncidents: [] },
    isLoading: false,
    isError: false,
    error: null,
    refetch: jest.fn(),
    ...overrides,
  } as unknown as ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>);
};

const optionValues = (
  builder: RepairQueueFilterBuilder | undefined,
): string[] =>
  builder instanceof StringListFilterCategoryBuilder
    ? (builder.options?.map((o) => o.value) ?? [])
    : [];

describe('useRepairQueueFilterBuilders', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockIrmIncidents();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('sources each core column from the dimension key the repopulate cron reads', () => {
    // Mirrors the real response shape: the Devices table contributes the base
    // dimensions, everything else arrives under labels.
    mockDimensions({
      baseDimensions: {
        id: { values: ['chromeos1-row2-rack3-host4'] },
        state: { values: ['ready'] },
      },
      labels: {
        'label-board': { values: ['brya', 'nami'] },
        'label-model': { values: ['volteer', 'corsola'] },
        'label-pool': { values: ['camerabox-support', 'DUT_POOL_QUOTA'] },
        dut_state: { values: ['needs_manual_repair', 'repair_failed'] },
        'label-servo_state': { values: ['working', 'broken'] },
      },
    });

    const { result } = renderFilterBuilders();

    expect(result.current.isLoading).toBe(false);

    const builders = result.current.filterBuilders;

    // The base dimension `id` is not exposed directly; it is mapped to `device_id`.
    expect(builders.id).toBeUndefined();

    expect(optionValues(builders.device_id)).toEqual([
      BLANK_VALUE,
      'chromeos1-row2-rack3-host4',
    ]);
    expect(optionValues(builders.board)).toEqual([BLANK_VALUE, 'brya', 'nami']);
    expect(optionValues(builders.model)).toEqual([
      BLANK_VALUE,
      'volteer',
      'corsola',
    ]);
    expect(optionValues(builders.pools)).toEqual([
      BLANK_VALUE,
      'camerabox-support',
      'DUT_POOL_QUOTA',
    ]);
    expect(optionValues(builders.dut_state)).toEqual([
      BLANK_VALUE,
      'needs_manual_repair',
      'repair_failed',
    ]);

    expect(builders.device_id.label).toBe('ID');
    expect(builders.board.label).toBe('Board');
    expect(builders.model.label).toBe('Model');
    expect(builders.pools.label).toBe('Pool');
    expect(builders.dut_state.label).toBe('State');

    // Non-core labels stay in the list, core ones are deduplicated away.
    expect(builders['labels."label-servo_state"']).toBeDefined();
    expect(builders['labels."label-pool"']).toBeUndefined();
    expect(builders['labels."label-model"']).toBeUndefined();
    expect(builders['labels."label-board"']).toBeUndefined();
  });

  it('handles empty dimensions gracefully', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: undefined,
        isLoading: true,
        isPending: true,
        isError: false,
        error: null,
      } as unknown as ReturnType<
        typeof UseDeviceDimensionsModule.useDeviceDimensions
      >);

    const { result } = renderFilterBuilders();

    expect(result.current.isLoading).toBe(true);

    const builders = result.current.filterBuilders;
    expect(builders.device_id).toBeDefined();
    expect(builders.id).toBeUndefined();
    expect(builders.pools).toBeDefined();
    expect(builders.model).toBeDefined();
    expect(builders.board).toBeDefined();
    expect(builders.dut_state).toBeDefined();
  });

  it('offers the normalized peripheral columns with a fixed enum', () => {
    mockDimensions({
      baseDimensions: {},
      labels: { 'label-servo_state': { values: ['SERVOD_ISSUE'] } },
    });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    for (const key of ['servo_state', 'wifi_state', 'bluetooth_state']) {
      const values = optionValues(builders[key]);
      expect(values).toEqual([
        BLANK_VALUE,
        'OK',
        'BROKEN',
        'MISSING',
        'NOT_APPLICABLE',
      ]);
      // The columns never store PERIPHERAL_STATE_UNSPECIFIED, so a rule on
      // UNKNOWN would score 0 forever.
      expect(values).not.toContain('UNKNOWN');
    }

    // The raw label survives alongside the normalized column. They target
    // different values, so both are useful.
    expect(builders['labels."label-servo_state"']).toBeDefined();
    expect(builders.servo_state.label).toBe('Servo');
    expect(builders.wifi_state.label).toBe('Wi-Fi');
    expect(builders.bluetooth_state.label).toBe('Bluetooth');
  });

  it('does not offer claimed_by as a rule filter', () => {
    mockDimensions({ baseDimensions: {}, labels: {} });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    // Ranking on who claimed a task inverts the point of the queue, and the
    // set of assignees is unbounded, so there is nothing sensible to offer.
    expect(builders.claimed_by).toBeUndefined();
  });

  it('drops dut_name, dut_id, and id, which duplicate the ID column', () => {
    mockDimensions({
      baseDimensions: { id: { values: ['dut-1'] } },
      labels: {
        id: { values: ['dut-1'] },
        dut_name: { values: ['dut-1'] },
        dut_id: { values: ['dut-1'] },
        'label-phase': { values: ['MP'] },
      },
    });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    expect(builders['labels."id"']).toBeUndefined();
    expect(builders['labels."dut_name"']).toBeUndefined();
    expect(builders['labels."dut_id"']).toBeUndefined();
    expect(builders['labels."label-phase"']).toBeDefined();
  });

  it('skips label keys with no values anywhere in the fleet', () => {
    mockDimensions({
      baseDimensions: {},
      labels: {
        'label-empty': { values: [] },
        'label-phase': { values: ['MP'] },
      },
    });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    expect(builders['labels."label-empty"']).toBeUndefined();
    expect(builders['labels."label-phase"']).toBeDefined();
  });

  it('offers pool_health_pct and model_health_pct as range filter category builders', () => {
    mockDimensions({ baseDimensions: {}, labels: {} });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    const poolHealthBuilder = builders[
      POOL_HEALTH_FILTER_KEY
    ] as RangeFilterCategoryBuilder;
    expect(poolHealthBuilder).toBeInstanceOf(RangeFilterCategoryBuilder);
    expect(poolHealthBuilder.label).toBe('Pool Health');
    expect(poolHealthBuilder.min).toBe(0);
    expect(poolHealthBuilder.max).toBe(100);
    expect(poolHealthBuilder.scaleFactor).toBe(0.01);
    expect(poolHealthBuilder.unit).toBe('%');

    const modelHealthBuilder = builders[
      MODEL_HEALTH_FILTER_KEY
    ] as RangeFilterCategoryBuilder;
    expect(modelHealthBuilder).toBeInstanceOf(RangeFilterCategoryBuilder);
    expect(modelHealthBuilder.label).toBe('Model Health');
    expect(modelHealthBuilder.min).toBe(0);
    expect(modelHealthBuilder.max).toBe(100);
    expect(modelHealthBuilder.scaleFactor).toBe(0.01);
    expect(modelHealthBuilder.unit).toBe('%');
  });

  it('builds Active IRM filter with deduplicated bug IDs in useRepairQueueFilterBuilders', () => {
    mockDimensions({ baseDimensions: {}, labels: {} });
    mockIrmIncidents({
      data: {
        irmIncidents: [
          {
            id: '1',
            masterBugId: '12345678',
            title: 'Volteer issue',
            expressionsAip160: [],
            affectedDevicesCount: 0,
          },
          {
            id: '2',
            masterBugId: '  12345678  ',
            title: 'Duplicate bug',
            expressionsAip160: [],
            affectedDevicesCount: 0,
          },
          {
            id: '3',
            masterBugId: '87654321',
            title: 'Second issue',
            expressionsAip160: [],
            affectedDevicesCount: 0,
          },
        ],
      },
    });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    expect(builders[ACTIVE_IRM_FILTER_KEY]).toBeDefined();
    expect(builders[ACTIVE_IRM_FILTER_KEY].label).toBe('Active IRM');
    expect(optionValues(builders[ACTIVE_IRM_FILTER_KEY])).toEqual([
      BLANK_VALUE,
      '12345678',
      '87654321',
    ]);
  });

  it('reports isLoading true when IRM incidents are still loading', () => {
    mockDimensions({ baseDimensions: {}, labels: {} });
    mockIrmIncidents({ data: undefined, isLoading: true });

    const { result } = renderFilterBuilders();
    expect(result.current.isLoading).toBe(true);
  });
});

describe('useIrmIncidentFilterBuilders', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('offers device dimensions and labels while excluding health metrics and active_irm_ids', () => {
    mockDimensions({
      baseDimensions: { id: { values: ['dut-1'] } },
      labels: {
        'label-board': { values: ['brya'] },
        'label-phase': { values: ['MP'] },
      },
    });
    const useIrmSpy = jest.spyOn(UseIrmIncidentsModule, 'useIrmIncidents');

    const { result } = renderHook(() => useIrmIncidentFilterBuilders(), {
      wrapper,
    });
    const builders = result.current.filterBuilders;

    expect(result.current.isLoading).toBe(false);
    expect(useIrmSpy).not.toHaveBeenCalled();
    expect(optionValues(builders.device_id)).toEqual([BLANK_VALUE, 'dut-1']);
    expect(optionValues(builders.board)).toEqual([BLANK_VALUE, 'brya']);
    expect(builders['labels."label-phase"']).toBeDefined();
    expect(builders[POOL_HEALTH_FILTER_KEY]).toBeUndefined();
    expect(builders[MODEL_HEALTH_FILTER_KEY]).toBeUndefined();
    expect(builders[ACTIVE_IRM_FILTER_KEY]).toBeUndefined();
  });
});
