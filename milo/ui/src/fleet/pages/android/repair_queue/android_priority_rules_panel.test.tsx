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
import { fireEvent, render, renderHook, screen } from '@testing-library/react';

import * as UseAdminTaskPermissionModule from '@/fleet/components/actions/shared/use_admin_task_permission';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import * as UseAndroidPriorityRulesModule from '@/fleet/pages/android/repair_queue/use_android_priority_rules';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { createMockUseFleetConsoleClient } from '@/fleet/testing_tools/mocks/fleet_console_client';
import { AndroidWorkspace } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidPriorityRulesPanel } from './android_priority_rules_panel';
import * as UseAndroidPriorityRuleFilterBuildersModule from './use_android_priority_rule_filter_builders';
import { useAndroidPriorityRuleFilterBuilders } from './use_android_priority_rule_filter_builders';

describe('<AndroidPriorityRulesPanel /> and filter builders', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    jest
      .spyOn(PrpcClientsModule, 'useFleetConsoleClient')
      .mockImplementation(createMockUseFleetConsoleClient());
    jest
      .spyOn(UseAdminTaskPermissionModule, 'usePriorityRulesPermission')
      .mockReturnValue({
        hasPermission: true,
        fetchPermissions: jest.fn(),
        isError: false,
        error: null,
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('offers only repair task fields, quotes dynamic label keys and drops pte_labs for Android', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {
            id: { values: ['dev-1'] },
            host_group: { values: ['group-a', 'pte_labs'] },
            run_target: { values: ['seahawk'] },
            state: { values: ['MISSING'] },
            claimed_by: { values: ['user@example.com'] },
            lab_name: { values: ['lab-a'] },
            type: { values: ['type-should-be-excluded'] },
            hostname: { values: ['host-should-be-excluded'] },
          },
          labels: {
            custom_label: { values: ['val1'] },
          },
        },
        isLoading: false,
      } as unknown as ReturnType<
        typeof UseDeviceDimensionsModule.useDeviceDimensions
      >);

    const { result } = renderHook(() =>
      useAndroidPriorityRuleFilterBuilders('Android'),
    );

    const keys = Object.keys(result.current.filterBuilders);
    expect(keys).toContain('device_id');
    expect(keys).toContain('lab_name');
    expect(keys).toContain('host_group');
    expect(keys).toContain('run_target');
    expect(keys).toContain('state');
    expect(keys).toContain('claimed_by');
    expect(keys).toContain('fc_machine_type');
    expect(keys).toContain('labels."custom_label"');
    expect(keys).not.toContain('lab');
    expect(keys).not.toContain('type');
    expect(keys).not.toContain('hostname');

    expect(keys).toContain('host_group_health_pct');
    expect(keys).toContain('run_target_health_pct');

    const hostGroupBuilder = result.current.filterBuilders.host_group;
    const hostGroupValues = (
      hostGroupBuilder instanceof StringListFilterCategoryBuilder
        ? (hostGroupBuilder.options ?? [])
        : []
    ).map((o) => o.value);
    expect(hostGroupValues).toContain('group-a');
    expect(hostGroupValues).not.toContain('pte_labs');
  });

  it('hides host_group dimension for Pixel workspace', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {
            id: { values: ['dev-1'] },
            host_group: { values: ['pte_labs'] },
            run_target: { values: ['seahawk'] },
            state: { values: ['MISSING'] },
            claimed_by: { values: ['user@example.com'] },
            lab_name: { values: ['lab-a'] },
          },
          labels: {
            custom_label: { values: ['val1'] },
          },
        },
        isLoading: false,
      } as unknown as ReturnType<
        typeof UseDeviceDimensionsModule.useDeviceDimensions
      >);

    const { result } = renderHook(() =>
      useAndroidPriorityRuleFilterBuilders('Pixel'),
    );

    const keys = Object.keys(result.current.filterBuilders);
    expect(keys).toContain('device_id');
    expect(keys).toContain('lab_name');
    expect(keys).not.toContain('host_group');
    expect(keys).toContain('run_target');
    expect(keys).toContain('state');
    expect(keys).toContain('claimed_by');
    expect(keys).toContain('fc_machine_type');
    expect(keys).toContain('labels."custom_label"');
  });

  it('renders empty state and allows adding a draft rule', () => {
    jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue({
        rules: [],
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: jest.fn(),
        isCreating: false,
        createError: null,
        updateRule: jest.fn(),
        isUpdating: false,
        updateError: null,
        deleteRule: jest.fn(),
        isDeleting: false,
        deleteError: null,
      });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Android" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText(/No priority scoring rules configured/i),
    ).toBeInTheDocument();

    const addBtn = screen.getByTestId('add-priority-rule-button');
    fireEvent.click(addBtn);

    expect(screen.getByLabelText('Rule 1 points weight')).toBeInTheDocument();
  });

  it('does not overwrite dirty user edits when rules re-sync', () => {
    const makeRules = (weight: string) => [
      {
        id: 'rule-1',
        expressionAip160: 'run_target = "seahawk"',
        weight,
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
      },
    ];
    const makeHookResult = (weight: string) => ({
      rules: makeRules(weight),
      isLoading: false,
      isError: false,
      error: null,
      refetch: jest.fn(),
      createRule: jest.fn(),
      isCreating: false,
      createError: null,
      updateRule: jest.fn(),
      isUpdating: false,
      updateError: null,
      deleteRule: jest.fn(),
      isDeleting: false,
      deleteError: null,
    });

    const mockUseRules = jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue(makeHookResult('10'));

    const ui = (
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Android" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(ui);

    const weightInput = screen.getByTestId(
      'rule-weight-input-rule-1',
    ) as HTMLInputElement;
    expect(weightInput.value).toBe('10');

    fireEvent.change(weightInput, { target: { value: '42' } });
    expect(weightInput.value).toBe('42');
    expect(screen.getByTestId('rule-apply-button-rule-1')).toBeInTheDocument();

    // A refetch (e.g. the 10s poll) returns a new rules array with the
    // backend value: the dirty draft must survive.
    mockUseRules.mockReturnValue(makeHookResult('10'));
    rerender(ui);
    expect(
      (screen.getByTestId('rule-weight-input-rule-1') as HTMLInputElement)
        .value,
    ).toBe('42');
    expect(screen.getByTestId('rule-apply-button-rule-1')).toBeInTheDocument();
  });

  it('calls useAndroidPriorityRules with the provided workspace', () => {
    const mockUseRules = jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue({
        rules: [],
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: jest.fn(),
        isCreating: false,
        createError: null,
        updateRule: jest.fn(),
        isUpdating: false,
        updateError: null,
        deleteRule: jest.fn(),
        isDeleting: false,
        deleteError: null,
      });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Pixel" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(mockUseRules).toHaveBeenCalledWith('Pixel');
  });

  it('hides edit controls when user does not have priority rules permission', () => {
    jest
      .spyOn(UseAdminTaskPermissionModule, 'usePriorityRulesPermission')
      .mockReturnValue({
        hasPermission: false,
        fetchPermissions: jest.fn(),
        isError: false,
        error: null,
      });

    jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue({
        rules: [
          {
            id: 'rule-1',
            expressionAip160: 'run_target = "seahawk"',
            weight: '10',
            workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
          },
        ],
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: jest.fn(),
        isCreating: false,
        createError: null,
        updateRule: jest.fn(),
        isUpdating: false,
        updateError: null,
        deleteRule: jest.fn(),
        isDeleting: false,
        deleteError: null,
      });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Android" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.queryByTestId('add-priority-rule-button'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('rule-delete-button-rule-1'),
    ).not.toBeInTheDocument();
    const weightInput = screen.getByTestId(
      'rule-weight-input-rule-1',
    ) as HTMLInputElement;
    expect(weightInput).toBeDisabled();
  });

  it('wires workspace-scoped filter builders and the Android filter placeholder', () => {
    const mockUseFilterBuilders = jest
      .spyOn(
        UseAndroidPriorityRuleFilterBuildersModule,
        'useAndroidPriorityRuleFilterBuilders',
      )
      .mockReturnValue({ filterBuilders: {}, isLoading: false });

    jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue({
        rules: [
          {
            id: 'rule-1',
            expressionAip160: '',
            weight: '10',
            workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
          },
        ],
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: jest.fn(),
        isCreating: false,
        createError: null,
        updateRule: jest.fn(),
        isUpdating: false,
        updateError: null,
        deleteRule: jest.fn(),
        isDeleting: false,
        deleteError: null,
      });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Pixel" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(mockUseFilterBuilders).toHaveBeenCalledWith('Pixel');
    expect(
      screen.getByPlaceholderText(
        'Add rule filter (e.g. state = "DYING" AND run_target = "seahawk")...',
      ),
    ).toBeInTheDocument();
  });

  it('renders a saved health percentage rule as a percentage chip', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: { baseDimensions: {}, labels: {} },
        isLoading: false,
      } as unknown as ReturnType<
        typeof UseDeviceDimensionsModule.useDeviceDimensions
      >);
    jest
      .spyOn(UseAndroidPriorityRulesModule, 'useAndroidPriorityRules')
      .mockReturnValue({
        rules: [
          {
            id: 'rule-1',
            expressionAip160: 'host_group_health_pct <= 0.5',
            weight: '30',
            workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
          },
        ],
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: jest.fn(),
        isCreating: false,
        createError: null,
        updateRule: jest.fn(),
        isUpdating: false,
        updateError: null,
        deleteRule: jest.fn(),
        isDeleting: false,
        deleteError: null,
      });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidPriorityRulesPanel workspace="Android" />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText('[ Host Group Health ]: to 50'),
    ).toBeInTheDocument();
  });
});
