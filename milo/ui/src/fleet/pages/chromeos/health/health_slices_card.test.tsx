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
import { act, fireEvent, render, screen } from '@testing-library/react';
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
    reorderSlicesMutation: fakeMutation((sliceIds: readonly string[]) =>
      setSlices((prev) => {
        const orderMap = new Map(sliceIds.map((id, index) => [id, index]));
        return [...prev].sort(
          (a, b) => (orderMap.get(a.id) ?? 999) - (orderMap.get(b.id) ?? 999),
        );
      }),
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

  it('renders ribbon with Add Slice button when no slices are configured', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('health-slices-card')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /^add slice$/i }),
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
    expect(screen.getByTestId('slice-pct-1')).toHaveTextContent('95%');

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

    // Open edit dialog via kebab menu
    fireEvent.click(screen.getByLabelText('More options for To Delete'));
    fireEvent.click(screen.getByText('Edit slice'));
    expect(screen.getByText('Edit Health Slice')).toBeInTheDocument();

    // Click delete inside modal
    fireEvent.click(screen.getByRole('button', { name: /delete slice/i }));

    expect(screen.queryByText('To Delete')).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /^add slice$/i }),
    ).toBeInTheDocument();
  });

  it('allows deleting a slice directly from the card kebab menu', () => {
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
    fireEvent.change(titleInput, { target: { value: 'Direct Delete' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(screen.getByText('Direct Delete')).toBeInTheDocument();

    // Open kebab menu and click delete
    fireEvent.click(screen.getByLabelText('More options for Direct Delete'));
    fireEvent.click(screen.getByText('Delete slice'));

    expect(screen.queryByText('Direct Delete')).not.toBeInTheDocument();
  });

  it('combines global filter with slice criteria when active', () => {
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

    // Add a slice to verify combined query
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    const titleInput = screen.getByLabelText('Slice Title');
    fireEvent.change(titleInput, { target: { value: 'Brya Fleet' } });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    expect(trendsSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        grouping: TrendlineGrouping.GROUP_BY_OVERALL,
        startTime: expect.stringMatching(
          /^\d{4}-\d{2}-\d{2}T\d{2}:00:00\.000Z$/,
        ),
        endTime: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T\d{2}:00:00\.000Z$/),
        filter: expect.stringContaining('model = "volteer"'),
      }),
    );
  });

  it('expands and collapses floating overlay tray with 5+ slices', async () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // Add 5 slices so the expand trigger button appears
    for (let i = 1; i <= 5; i++) {
      fireEvent.click(screen.getByRole('button', { name: /^add slice$/i }));
      fireEvent.change(screen.getByLabelText('Slice Title'), {
        target: { value: `Slice ${i}` },
      });
      fireEvent.click(screen.getByRole('button', { name: /save slice/i }));
    }

    const expandBtn = screen.getByTestId('expand-slices-button');
    expect(expandBtn).toHaveTextContent('All Slices (5)');

    // 2. Click expand to open the floating tray
    fireEvent.click(expandBtn);
    expect(screen.getByTestId('slices-overlay-tray')).toBeInTheDocument();
    expect(
      screen.queryByTestId('collapse-slices-button'),
    ).not.toBeInTheDocument();

    // Inside the expanded tray, the Add Slice dashed card is also available
    expect(
      screen.getByRole('button', { name: /^add slice$/i }),
    ).toBeInTheDocument();

    // Wait for ClickAwayListener to activate
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });

    // 3. Click outside (click away) to close the tray
    fireEvent.click(document.body);
    expect(screen.queryByTestId('slices-overlay-tray')).not.toBeInTheDocument();
  });

  it('selects slice inside overlay tray and auto-collapses tray', () => {
    const onSelectSlice = jest.fn();

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard onSelectSlice={onSelectSlice} />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // 1. Add 5 slices
    for (let i = 1; i <= 5; i++) {
      fireEvent.click(screen.getByRole('button', { name: /^add slice$/i }));
      fireEvent.change(screen.getByLabelText('Slice Title'), {
        target: { value: `Slice ${i}` },
      });
      fireEvent.click(screen.getByRole('button', { name: /save slice/i }));
    }

    // 2. Open tray
    fireEvent.click(screen.getByTestId('expand-slices-button'));
    expect(screen.getByTestId('slices-overlay-tray')).toBeInTheDocument();

    // 3. Click on Slice 2 inside tray
    const slice2Item = screen.getByTestId('health-slice-item-2');
    fireEvent.click(slice2Item);

    // Verify onSelectSlice fired and tray closed
    expect(onSelectSlice).toHaveBeenCalledWith(
      expect.objectContaining<Partial<HealthSlice>>({ name: 'Slice 2' }),
    );
    expect(screen.queryByTestId('slices-overlay-tray')).not.toBeInTheDocument();
  });

  it('adaptively handles 1-3 slices, 4 slices, and 5+ slices layout', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // Initially 0 slices: dashed Add Slice card in grid, no divider
    expect(
      screen.getByRole('button', { name: /^add slice$/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('add-slice-divider-button'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('expand-slices-button'),
    ).not.toBeInTheDocument();

    // Add 3 slices
    for (let i = 1; i <= 3; i++) {
      fireEvent.click(screen.getByRole('button', { name: /^add slice$/i }));
      fireEvent.change(screen.getByLabelText('Slice Title'), {
        target: { value: `Slice ${i}` },
      });
      fireEvent.click(screen.getByRole('button', { name: /save slice/i }));
    }

    // With 3 slices: still dashed Add Slice card in grid, no divider
    expect(
      screen.getByRole('button', { name: /^add slice$/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('add-slice-divider-button'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('expand-slices-button'),
    ).not.toBeInTheDocument();

    // Add 4th slice
    fireEvent.click(screen.getByRole('button', { name: /^add slice$/i }));
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice 4' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    // With 4 slices: divider has Add Slice button; dashed card is NOT in grid
    const addSliceDividerBtn = screen.getByTestId('add-slice-divider-button');
    expect(addSliceDividerBtn).toBeInTheDocument();
    expect(
      screen.queryByTestId('expand-slices-button'),
    ).not.toBeInTheDocument();

    // Clicking divider Add Slice button opens modal
    fireEvent.click(addSliceDividerBtn);
    expect(screen.getByText('Add New Health Slice')).toBeInTheDocument();

    // Add 5th slice from modal
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice 5' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    // With 5 slices: divider has All Slices (5) button; Add Slice divider button is gone
    expect(
      screen.queryByTestId('add-slice-divider-button'),
    ).not.toBeInTheDocument();
    const expandBtn = screen.getByTestId('expand-slices-button');
    expect(expandBtn).toHaveTextContent('All Slices (5)');
  });

  it('keeps cards in their stable position when clicked (no unexpected reordering)', () => {
    const onSelectSlice = jest.fn();

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard onSelectSlice={onSelectSlice} />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // 1. Add two slices
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice Alpha' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice Beta' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    // Verify initial order: Alpha is first, Beta is second
    let sliceItems = screen.getAllByTestId(/^health-slice-item-/);
    expect(sliceItems[0]).toHaveTextContent('Slice Alpha');
    expect(sliceItems[1]).toHaveTextContent('Slice Beta');

    // Click on Slice Beta (card #2)
    fireEvent.click(sliceItems[1]);
    expect(onSelectSlice).toHaveBeenCalled();

    // Verify order remains unchanged: Alpha is still first, Beta is still second!
    sliceItems = screen.getAllByTestId(/^health-slice-item-/);
    expect(sliceItems[0]).toHaveTextContent('Slice Alpha');
    expect(sliceItems[1]).toHaveTextContent('Slice Beta');
  });

  it('reorders slices via drag and drop', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    // 1. Add two slices
    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice One' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    fireEvent.click(screen.getByRole('button', { name: /add slice/i }));
    fireEvent.change(screen.getByLabelText('Slice Title'), {
      target: { value: 'Slice Two' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save slice/i }));

    let sliceItems = screen.getAllByTestId(/^health-slice-item-/);
    expect(sliceItems[0]).toHaveTextContent('Slice One');
    expect(sliceItems[1]).toHaveTextContent('Slice Two');

    // 2. Drag Slice Two and drop it onto Slice One
    const dataTransfer = {
      setData: jest.fn(),
      effectAllowed: 'move',
      dropEffect: 'move',
    };

    fireEvent.dragStart(sliceItems[1], { dataTransfer });
    fireEvent.dragOver(sliceItems[0], { dataTransfer });
    fireEvent.drop(sliceItems[0], { dataTransfer });
    fireEvent.dragEnd(sliceItems[1]);

    // 3. Verify Slice Two is now first, and Slice One is second
    sliceItems = screen.getAllByTestId(/^health-slice-item-/);
    expect(sliceItems[0]).toHaveTextContent('Slice Two');
    expect(sliceItems[1]).toHaveTextContent('Slice One');
  });

  it('debounces backend persistence of slice order during drag and drop', () => {
    jest.useFakeTimers();
    try {
      const mockReorder = jest.fn();
      jest.spyOn(UseHealthSlicesModule, 'useHealthSlices').mockReturnValue({
        slicesQuery: {
          data: {
            healthSlices: [
              { id: '1', name: 'Alpha', filter: '', displayOrder: 0 },
              { id: '2', name: 'Beta', filter: '', displayOrder: 1 },
            ],
          },
          isPending: false,
          isError: false,
          error: null,
        },
        createSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        updateSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        deleteSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        reorderSlicesMutation: {
          mutate: mockReorder,
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
      } as unknown as UseHealthSlicesResult);

      render(
        <FakeContextProvider>
          <ShortcutProvider>
            <HealthSlicesCard />
          </ShortcutProvider>
        </FakeContextProvider>,
      );

      const sliceItems = screen.getAllByTestId(/^health-slice-item-/);
      const dataTransfer = {
        setData: jest.fn(),
        effectAllowed: 'move',
        dropEffect: 'move',
      };

      fireEvent.dragStart(sliceItems[1], { dataTransfer });
      fireEvent.dragOver(sliceItems[0], { dataTransfer });
      fireEvent.drop(sliceItems[0], { dataTransfer });
      fireEvent.dragEnd(sliceItems[1]);

      // Immediately after drop: mutation has NOT been called yet due to debounce
      expect(mockReorder).not.toHaveBeenCalled();

      // Advance by 400ms: still not called
      jest.advanceTimersByTime(400);
      expect(mockReorder).not.toHaveBeenCalled();

      // Advance past 500ms debounce threshold
      jest.advanceTimersByTime(200);
      expect(mockReorder).toHaveBeenCalledWith(['2', '1']);
    } finally {
      jest.useRealTimers();
    }
  });

  it('flushes pending reorder mutation when unmounting during debounce window', () => {
    jest.useFakeTimers();
    try {
      const mockReorder = jest.fn();
      jest.spyOn(UseHealthSlicesModule, 'useHealthSlices').mockReturnValue({
        slicesQuery: {
          data: {
            healthSlices: [
              { id: '1', name: 'Alpha', filter: '', displayOrder: 0 },
              { id: '2', name: 'Beta', filter: '', displayOrder: 1 },
            ],
          },
          isPending: false,
          isError: false,
          error: null,
        },
        createSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        updateSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        deleteSliceMutation: {
          mutate: jest.fn(),
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
        reorderSlicesMutation: {
          mutate: mockReorder,
          isPending: false,
          reset: jest.fn(),
          error: null,
        },
      } as unknown as UseHealthSlicesResult);

      const { unmount } = render(
        <FakeContextProvider>
          <ShortcutProvider>
            <HealthSlicesCard />
          </ShortcutProvider>
        </FakeContextProvider>,
      );

      const sliceItems = screen.getAllByTestId(/^health-slice-item-/);
      const dataTransfer = {
        setData: jest.fn(),
        effectAllowed: 'move',
        dropEffect: 'move',
      };

      fireEvent.dragStart(sliceItems[1], { dataTransfer });
      fireEvent.dragOver(sliceItems[0], { dataTransfer });
      fireEvent.drop(sliceItems[0], { dataTransfer });
      fireEvent.dragEnd(sliceItems[1]);

      // Mutation has not fired yet
      expect(mockReorder).not.toHaveBeenCalled();

      // Unmount before debounce timer finishes
      unmount();

      // Flushed immediately on cleanup
      expect(mockReorder).toHaveBeenCalledWith(['2', '1']);
    } finally {
      jest.useRealTimers();
    }
  });

  it('renders error alert when reorder mutation fails', () => {
    const mockReset = jest.fn();
    jest.spyOn(UseHealthSlicesModule, 'useHealthSlices').mockReturnValue({
      slicesQuery: {
        data: {
          healthSlices: [
            { id: '1', name: 'Alpha', filter: '', displayOrder: 0 },
          ],
        },
        isPending: false,
        isError: false,
        error: null,
      },
      createSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      updateSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      deleteSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      reorderSlicesMutation: {
        mutate: jest.fn(),
        isPending: false,
        isError: true,
        reset: mockReset,
        error: new Error('Database transaction lock conflict'),
      },
    } as unknown as UseHealthSlicesResult);

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        'Failed to save slice order: Database transaction lock conflict',
      ),
    ).toBeInTheDocument();

    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);
    expect(mockReset).toHaveBeenCalled();
  });

  it('auto-purges stale defaultSliceId when server slices do not contain it', () => {
    localStorage.setItem(
      UseHealthSlicesModule.DEFAULT_SLICE_STORAGE_KEY,
      'deleted-slice-999',
    );

    jest.spyOn(UseHealthSlicesModule, 'useHealthSlices').mockReturnValue({
      slicesQuery: {
        data: {
          healthSlices: [
            { id: '1', name: 'Alpha', filter: '', displayOrder: 0 },
          ],
        },
        isPending: false,
        isError: false,
        error: null,
      },
      createSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      updateSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      deleteSliceMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
      reorderSlicesMutation: {
        mutate: jest.fn(),
        isPending: false,
        reset: jest.fn(),
        error: null,
      },
    } as unknown as UseHealthSlicesResult);

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthSlicesCard />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(
      localStorage.getItem(UseHealthSlicesModule.DEFAULT_SLICE_STORAGE_KEY),
    ).toBeNull();
  });
});
