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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react';

import * as UseAdminTaskPermissionModule from '@/fleet/components/actions/shared/use_admin_task_permission';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import {
  IrmIncident,
  ListIrmIncidentsResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ActiveIrmTable } from './active_irm_table';
import * as UseIrmIncidentFilterBuildersModule from './use_irm_incident_filter_builders';
import * as UseIrmIncidentsModule from './use_irm_incidents';
import { REPAIR_QUEUE_QUERY_KEY } from './use_repair_queue';

const MOCK_INCIDENTS: readonly IrmIncident[] = [
  {
    id: '1',
    masterBugId: '  12345678  ',
    title: 'ChromeOS fleet wide lab issue',
    expressionsAip160: ['board = "volteer"'],
    affectedDevicesCount: 4,
  },
  {
    id: '2',
    masterBugId: '',
    title: '',
    expressionsAip160: [],
    affectedDevicesCount: 0,
  },
  {
    id: '3',
    masterBugId: '33333333',
    title: 'Global lab outage',
    expressionsAip160: [''],
    affectedDevicesCount: 10,
  },
];

describe('<ActiveIrmTable />', () => {
  const mockUpdateIncident = jest.fn();
  const mockIsSyncingIncident = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    mockUpdateIncident.mockResolvedValue({
      irmIncident: {
        id: '1',
        masterBugId: '12345678',
        title: 'ChromeOS fleet wide lab issue',
        expressionsAip160: ['board = "volteer"'],
        affectedDevicesCount: 4,
      },
    });
    mockIsSyncingIncident.mockReturnValue(false);

    jest
      .spyOn(UseAdminTaskPermissionModule, 'usePriorityRulesPermission')
      .mockReturnValue({
        hasPermission: true,
        fetchPermissions: jest.fn().mockResolvedValue({ hasPermission: true }),
        isError: false,
        error: null,
      });

    jest
      .spyOn(UseIrmIncidentFilterBuildersModule, 'useIrmIncidentFilterBuilders')
      .mockReturnValue({
        filterBuilders: {
          board: new StringListFilterCategoryBuilder()
            .setLabel('Board')
            .setOptions([
              { label: 'volteer', value: 'volteer' },
              { label: 'brya', value: 'brya' },
            ]),
        },
        isLoading: false,
      });

    jest.spyOn(UseIrmIncidentsModule, 'useUpdateIrmIncident').mockReturnValue({
      updateIncident: mockUpdateIncident,
      isUpdating: false,
      updateError: null,
      syncingIncidentIds: new Set(),
      isSyncingIncident: mockIsSyncingIncident,
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const mockUseIrm = (
    overrides: Partial<
      ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>
    >,
  ) =>
    jest.spyOn(UseIrmIncidentsModule, 'useIrmIncidents').mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: jest.fn(),
      ...overrides,
    } as ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>);

  const renderComponent = () =>
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <ActiveIrmTable />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

  it('renders loading state', () => {
    mockUseIrm({ isLoading: true });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('renders error state', () => {
    mockUseIrm({
      isError: true,
      error: new Error('Failed to load IRM incidents'),
    });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(
      screen.getByText(/Failed to load IRM incidents/i),
    ).toBeInTheDocument();
  });

  it('renders empty state', () => {
    mockUseIrm({ data: { irmIncidents: [] } });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByText('No active IRM incidents')).toBeInTheDocument();
  });

  it('renders data, filter chips, affected devices count, and handles edge cases', () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByText('Bug ID')).toBeInTheDocument();
    expect(screen.getByText('Incident Name')).toBeInTheDocument();
    expect(screen.getByText('Filters')).toBeInTheDocument();
    expect(screen.getByText('Affected Devices')).toBeInTheDocument();

    const link = screen.getByRole('link', { name: /b\/12345678/i });
    expect(link).toHaveAttribute(
      'href',
      'https://b.corp.google.com/issues/12345678',
    );
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');

    expect(
      screen.getByText('ChromeOS fleet wide lab issue'),
    ).toBeInTheDocument();
    const row1 = screen.getByText('b/12345678').closest('tr')!;
    const row3 = screen.getByText('b/33333333').closest('tr')!;
    expect(within(row1).getByTestId('irm-filters-btn')).toHaveTextContent(
      '1 applied',
    );
    expect(within(row3).getByTestId('irm-filters-btn')).toHaveTextContent(
      'All devices',
    );
    expect(within(row1).getByTestId('irm-affected-count')).toHaveTextContent(
      '4',
    );
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('opens dialog in read-only mode for users without priority rules permission', () => {
    jest
      .spyOn(UseAdminTaskPermissionModule, 'usePriorityRulesPermission')
      .mockReturnValue({
        hasPermission: false,
        fetchPermissions: jest.fn().mockResolvedValue({ hasPermission: false }),
        isError: false,
        error: null,
      });

    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row1 = screen.getByText('b/12345678').closest('tr')!;
    const row2 = screen.getByText('N/A').closest('tr')!;
    const btn = within(row1).getByTestId('irm-filters-btn');
    expect(btn).toBeInTheDocument();
    expect(btn).toBeEnabled();
    expect(within(row2).getByTestId('irm-no-filters')).toHaveTextContent(
      'None',
    );

    fireEvent.click(btn);
    const dialog = screen.getByTestId('irm-filter-dialog');
    expect(dialog).toBeInTheDocument();
    expect(
      within(dialog).getByText('View Incident Filters'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('irm-filter-save')).not.toBeInTheDocument();
    expect(screen.queryByTestId('irm-filter-add-row')).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('irm-filter-delete-btn'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('irm-filter-close')).toBeInTheDocument();
  });

  it('allows saving with all filter rows deleted to unlink the incident', async () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row = screen.getByText('b/12345678').closest('tr')!;
    fireEvent.click(within(row).getByTestId('irm-filters-btn'));

    const dialog = screen.getByTestId('irm-filter-dialog');
    expect(dialog).toBeInTheDocument();
    expect(
      within(dialog).getByText('Edit Incident Filters'),
    ).toBeInTheDocument();

    const [filterRow] = screen.getAllByTestId('irm-filter-row');
    fireEvent.click(within(filterRow).getByTestId('irm-filter-delete-btn'));
    expect(screen.getByText(/No filters configured/i)).toBeInTheDocument();

    const saveButton = screen.getByTestId('irm-filter-save');
    expect(saveButton).toBeEnabled();
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateIncident).toHaveBeenCalledWith({
        id: '1',
        expressionsAip160: [],
      });
    });
  });

  it('opens dialog filter editor and sets universal match all devices via save', async () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row = screen.getByText('b/12345678').closest('tr')!;
    fireEvent.click(within(row).getByTestId('irm-filters-btn'));

    const dialog = screen.getByTestId('irm-filter-dialog');
    expect(dialog).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('irm-filter-all-btn'));
    expect(
      screen.getByText('Matches all devices across the fleet'),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('irm-filter-save'));

    await waitFor(() => {
      expect(mockUpdateIncident).toHaveBeenCalledWith({
        id: '1',
        expressionsAip160: [''],
      });
    });
  });

  it('opens dialog with all devices row when clicking All devices and allows deleting it', async () => {
    mockUseIrm({
      data: {
        irmIncidents: [
          {
            id: '1',
            masterBugId: '12345678',
            title: 'Outage incident',
            expressionsAip160: [''],
            affectedDevicesCount: 5,
          },
        ],
      },
    });
    renderComponent();

    const btn = screen.getByTestId('irm-filters-btn');
    expect(btn).toHaveTextContent('All devices');

    fireEvent.click(btn);

    const dialog = screen.getByTestId('irm-filter-dialog');
    expect(dialog).toBeInTheDocument();
    expect(
      screen.getByText('Matches all devices across the fleet'),
    ).toBeInTheDocument();

    // Deleting the all-devices row resets to empty
    fireEvent.click(screen.getByTestId('irm-filter-delete-btn'));
    expect(
      screen.queryByText('Matches all devices across the fleet'),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/No filters configured/i)).toBeInTheDocument();

    // Saving unlinks the incident
    fireEvent.click(screen.getByTestId('irm-filter-save'));
    await waitFor(() => {
      expect(mockUpdateIncident).toHaveBeenCalledWith({
        id: '1',
        expressionsAip160: [],
      });
    });
  });

  it('deletes one of multiple expressions and saves remaining', async () => {
    mockUseIrm({
      data: {
        irmIncidents: [
          {
            id: '1',
            masterBugId: '12345678',
            title: 'Multi-filter incident',
            expressionsAip160: ['board = "volteer"', 'model = "brya"'],
            affectedDevicesCount: 3,
          },
        ],
      },
    });
    renderComponent();

    const btn = screen.getByTestId('irm-filters-btn');
    expect(btn).toHaveTextContent('2 applied');

    fireEvent.click(btn);

    const dialog = screen.getByTestId('irm-filter-dialog');
    expect(dialog).toBeInTheDocument();
    expect(
      within(dialog).getByText('Edit Incident Filters'),
    ).toBeInTheDocument();

    // Delete row 1 (model = "brya")
    const filterRows = screen.getAllByTestId('irm-filter-row');
    fireEvent.click(within(filterRows[1]).getByTestId('irm-filter-delete-btn'));
    fireEvent.click(screen.getByTestId('irm-filter-save'));

    await waitFor(() => {
      expect(mockUpdateIncident).toHaveBeenCalledWith({
        id: '1',
        expressionsAip160: ['board = "volteer"'],
      });
    });
  });

  it('blocks saving with error when an empty filter row is present', async () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row = screen.getByText('b/12345678').closest('tr')!;
    fireEvent.click(within(row).getByTestId('irm-filters-btn'));

    // Add a new row which starts empty
    fireEvent.click(screen.getByTestId('irm-filter-add-row'));

    // Try to save with empty row
    fireEvent.click(screen.getByTestId('irm-filter-save'));

    expect(
      screen.getByText('Filter expression cannot be empty'),
    ).toBeInTheDocument();
    expect(mockUpdateIncident).not.toHaveBeenCalled();

    // Delete the empty row (row index 1)
    const filterRows = screen.getAllByTestId('irm-filter-row');
    fireEvent.click(within(filterRows[1]).getByTestId('irm-filter-delete-btn'));

    // Now save succeeds
    fireEvent.click(screen.getByTestId('irm-filter-save'));

    await waitFor(() => {
      expect(mockUpdateIncident).toHaveBeenCalledWith({
        id: '1',
        expressionsAip160: ['board = "volteer"'],
      });
    });
  });

  it('abandons changes when cancel is clicked', () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row = screen.getByText('b/12345678').closest('tr')!;
    fireEvent.click(within(row).getByTestId('irm-filters-btn'));

    // Delete existing row
    const [filterRow] = screen.getAllByTestId('irm-filter-row');
    fireEvent.click(within(filterRow).getByTestId('irm-filter-delete-btn'));

    // Click cancel
    fireEvent.click(screen.getByTestId('irm-filter-cancel'));

    expect(screen.queryByTestId('irm-filter-dialog')).not.toBeInTheDocument();
    expect(mockUpdateIncident).not.toHaveBeenCalled();
  });

  it('shows sync spinner and disables filter button while incident is syncing', () => {
    mockIsSyncingIncident.mockImplementation((id: string) => id === '1');
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    const row1 = screen.getByText('b/12345678').closest('tr')!;
    const row2 = screen.getByText('N/A').closest('tr')!;
    expect(within(row1).getByTestId('irm-sync-spinner')).toBeInTheDocument();
    expect(
      within(row2).queryByTestId('irm-sync-spinner'),
    ).not.toBeInTheDocument();
    expect(within(row1).getByTestId('irm-filters-btn')).toBeDisabled();
  });

  it('does not treat trailing empty string as all devices when specific filter is present', () => {
    mockUseIrm({
      data: {
        irmIncidents: [
          {
            id: '1',
            masterBugId: '12345678',
            title: 'Volteer issue with trailing empty string',
            expressionsAip160: ['board = "volteer"', ''],
            affectedDevicesCount: 2,
          },
        ],
      },
    });
    renderComponent();

    expect(screen.queryByText('All devices')).not.toBeInTheDocument();
    expect(screen.getByText('1 applied')).toBeInTheDocument();
  });

  it('displays tooltip with filter expressions on hover', async () => {
    mockUseIrm({
      data: {
        irmIncidents: [
          {
            id: '1',
            masterBugId: '12345678',
            title: 'Volteer issue',
            expressionsAip160: ['board = "volteer"', 'pool = "faft"'],
            affectedDevicesCount: 2,
          },
        ],
      },
    });
    renderComponent();

    const btn = screen.getByTestId('irm-filters-btn');
    fireEvent.mouseOver(btn);

    expect(await screen.findByText('board = "volteer"')).toBeInTheDocument();
    expect(screen.getByText('pool = "faft"')).toBeInTheDocument();
  });
});

describe('useUpdateIrmIncident', () => {
  it('optimistically updates ListIrmIncidents cache immediately and invalidates repair queue on settle', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const initialData: ListIrmIncidentsResponse = {
      irmIncidents: [
        {
          id: '1',
          masterBugId: '12345678',
          title: 'Volteer issue',
          expressionsAip160: [],
          affectedDevicesCount: 0,
        },
      ],
    };
    queryClient.setQueryData(
      [...UseIrmIncidentsModule.IRM_INCIDENTS_QUERY_KEY, {}],
      initialData,
    );

    let resolveRpc!: (value: { irmIncident: IrmIncident }) => void;
    const mockUpdateRpc = jest.fn(
      () =>
        new Promise<{ irmIncident: IrmIncident }>((resolve) => {
          resolveRpc = resolve;
        }),
    );

    jest.spyOn(PrpcClientsModule, 'useFleetConsoleClient').mockReturnValue({
      UpdateIrmIncident: mockUpdateRpc,
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>);

    const invalidateSpy = jest.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(
      () => UseIrmIncidentsModule.useUpdateIrmIncident(),
      {
        wrapper: ({ children }: { children: React.ReactNode }) => (
          <QueryClientProvider client={queryClient}>
            {children}
          </QueryClientProvider>
        ),
      },
    );

    let mutationPromise!: Promise<unknown>;
    await act(async () => {
      mutationPromise = result.current.updateIncident({
        id: '1',
        expressionsAip160: ['board = "volteer"'],
      });
    });

    // Verify immediate optimistic update while RPC is still pending
    const optimisticCache = queryClient.getQueryData<ListIrmIncidentsResponse>([
      ...UseIrmIncidentsModule.IRM_INCIDENTS_QUERY_KEY,
      {},
    ]);
    expect(optimisticCache?.irmIncidents[0].expressionsAip160).toEqual([
      'board = "volteer"',
    ]);
    expect(result.current.isSyncingIncident('1')).toBe(true);

    // Resolve RPC with server affectedDevicesCount
    await act(async () => {
      resolveRpc({
        irmIncident: {
          id: '1',
          masterBugId: '12345678',
          title: 'Volteer issue',
          expressionsAip160: ['board = "volteer"'],
          affectedDevicesCount: 7,
        },
      });
      await mutationPromise;
    });

    const settledCache = queryClient.getQueryData<ListIrmIncidentsResponse>([
      ...UseIrmIncidentsModule.IRM_INCIDENTS_QUERY_KEY,
      {},
    ]);
    expect(settledCache?.irmIncidents[0].affectedDevicesCount).toBe(7);
    expect(result.current.isSyncingIncident('1')).toBe(false);
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: UseIrmIncidentsModule.IRM_INCIDENTS_QUERY_KEY,
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: REPAIR_QUEUE_QUERY_KEY,
    });
  });
});
