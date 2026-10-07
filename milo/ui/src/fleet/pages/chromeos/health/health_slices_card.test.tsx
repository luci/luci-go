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

import { UseQueryResult } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import {
  GetFleetAvailabilityTrendsResponse,
  HealthSlice,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HealthSlicesCard } from './health_slices_card';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';
import * as UseHealthSlicesModule from './use_health_slices';

type UseHealthSlicesResult = ReturnType<
  typeof UseHealthSlicesModule.useHealthSlices
>;

/** In-memory stand-in for the ListHealthSlices/Create/Update/Delete RPCs. */
const useFakeHealthSlices = (): UseHealthSlicesResult => {
  const [slices, setSlices] = useState<readonly HealthSlice[]>([]);

  const fakeMutation = <T,>(apply: (arg: T) => void) => ({
    mutate: (arg: T, opts?: { onSuccess?: () => void }) => {
      apply(arg);
      opts?.onSuccess?.();
    },
    reset: () => {},
    isPending: false,
    error: null,
  });

  return {
    slicesQuery: {
      data: { healthSlices: slices },
      isPending: false,
      isError: false,
      error: null,
    },
    createSliceMutation: fakeMutation((s: HealthSlice) =>
      setSlices((prev) => [...prev, { ...s, id: String(prev.length + 1) }]),
    ),
    updateSliceMutation: fakeMutation((s: HealthSlice) =>
      setSlices((prev) => prev.map((p) => (p.id === s.id ? s : p))),
    ),
    deleteSliceMutation: fakeMutation((id: string) =>
      setSlices((prev) => prev.filter((p) => p.id !== id)),
    ),
  } as unknown as UseHealthSlicesResult;
};

describe('HealthSlicesCard', () => {
  const currentHourIso = DateTime.now().toUTC().startOf('hour').toISO()!;

  beforeEach(() => {
    jest.clearAllMocks();

    jest
      .spyOn(UseHealthSlicesModule, 'useHealthSlices')
      .mockImplementation(useFakeHealthSlices);

    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((request) => {
        const isQuota = request.filter?.includes('board = "brya"');
        const mockData: GetFleetAvailabilityTrendsResponse = {
          metricType: isQuota
            ? TrendlineMetricType.AVAILABILITY
            : TrendlineMetricType.HEALTH,
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 0.95 }],
            },
          ],
        };

        return {
          data: mockData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders empty state when no slices are configured', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Health Slices')).toBeInTheDocument();
    expect(
      screen.getByText('No health slices configured.'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /add your first slice/i }),
    ).toBeInTheDocument();
  });

  it('adds a new slice and renders availability without inline filter description', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));

    expect(screen.getByText('Add New Health Slice')).toBeInTheDocument();

    const titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'Skyrim Test Cluster' } });

    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(screen.queryByText('Add New Health Slice')).not.toBeInTheDocument();
    expect(screen.getByText('Skyrim Test Cluster')).toBeInTheDocument();
    expect(screen.getByText('95%')).toBeInTheDocument();

    // Verify no filter description below progress bar
    expect(screen.queryByText(/^Filter:/i)).not.toBeInTheDocument();
    // Verify no inline delete button on the card
    expect(
      screen.queryByLabelText('Delete Skyrim Test Cluster'),
    ).not.toBeInTheDocument();
  });

  it('disables saving when the title is empty', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));

    const saveBtn = screen.getByRole('button', { name: /save slice/i });
    expect(saveBtn).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: '   ' },
    });
    expect(saveBtn).toBeDisabled();
  });

  it('prevents duplicate slice names and displays error message', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // 1. Add first slice
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    let titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'Labstations' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(screen.getByText('Labstations')).toBeInTheDocument();

    // 2. Try to add a second slice with the same name (case-insensitive)
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'labstations' } });

    expect(
      screen.getByText('A slice with this name already exists.'),
    ).toBeInTheDocument();
    const saveBtn = screen.getByRole('button', { name: /save slice/i });
    expect(saveBtn).toBeDisabled();

    // Attempting to click does nothing
    fireEvent.click(saveBtn);
    expect(screen.getByText('Add New Health Slice')).toBeInTheDocument();
  });

  it('triggers onSelectSlice when clicking on a slice card', () => {
    const onSelectSlice = jest.fn();

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard onSelectSlice={onSelectSlice} />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // Add a slice
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    const titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'Labstations' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    // Click on slice card
    const sliceCard = screen.getByTestId(/^health-slice-item-/);
    fireEvent.click(sliceCard);

    expect(onSelectSlice).toHaveBeenCalledWith(
      expect.objectContaining<Partial<HealthSlice>>({
        name: 'Labstations',
      }),
    );
  });

  it('allows deleting a slice from inside the edit modal', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // Add a slice
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    const titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'To Delete' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(screen.getByText('To Delete')).toBeInTheDocument();

    // Open edit dialog
    fireEvent.click(screen.getByLabelText('Edit To Delete'));
    expect(screen.getByText('Edit Health Slice')).toBeInTheDocument();

    // Click delete inside modal
    fireEvent.click(screen.getByRole('button', { name: /delete slice/i }));

    expect(screen.queryByText('To Delete')).not.toBeInTheDocument();
    expect(
      screen.getByText('No health slices configured.'),
    ).toBeInTheDocument();
  });

  it('displays "+ 1 Global" badge and combines global filter when active', () => {
    const trendsSpy = jest.spyOn(
      UseFleetAvailabilityTrendsModule,
      'useFleetAvailabilityTrends',
    );

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard globalFilter='model = "volteer"' />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('+ 1 Global')).toBeInTheDocument();

    // Add a slice to verify combined query
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    const titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'Brya Fleet' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(trendsSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        grouping: TrendlineGrouping.GROUP_BY_OVERALL,
        filter: expect.stringContaining('model = "volteer"'),
      }),
    );
  });
});
