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

import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useRepairQueueFilterBuilders } from './use_repair_queue_filter_builders';

const renderFilterBuilders = () =>
  renderHook(() => useRepairQueueFilterBuilders(), {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <FakeContextProvider>{children}</FakeContextProvider>
    ),
  });

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

const optionValues = (
  builder: { options?: ReadonlyArray<{ value: string }> } | undefined,
): string[] => (builder?.options ?? []).map((o) => o.value);

describe('useRepairQueueFilterBuilders', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('sources each core column from the dimension key the repopulate cron reads', () => {
    // Mirrors the real response shape: the Devices table contributes the base
    // dimensions, everything else arrives under labels.
    mockDimensions({
      baseDimensions: {
        dut_id: { values: ['chromeos1-row2-rack3-host4'] },
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

    // The base dimension `id` is not a repair_tasks column, so it is not offered.
    expect(builders.id).toBeUndefined();

    expect(optionValues(builders.dut_id)).toEqual([
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

    expect(builders.dut_id.label).toBe('Dut ID');
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
    expect(builders.dut_id).toBeDefined();
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

  it('drops dut_name, which duplicates the dut_id column', () => {
    mockDimensions({
      baseDimensions: { dut_id: { values: ['dut-1'] } },
      labels: {
        dut_name: { values: ['dut-1'] },
        'label-phase': { values: ['MP'] },
      },
    });

    const builders = renderFilterBuilders().result.current.filterBuilders;

    expect(builders['labels."dut_name"']).toBeUndefined();
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
});
