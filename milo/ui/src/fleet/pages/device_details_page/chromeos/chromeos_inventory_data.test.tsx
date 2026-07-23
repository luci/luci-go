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

import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSInventoryData } from './chromeos_inventory_data';

// Mock react-query queries and mutations
const mockMutate = jest.fn();
let mutationOptions: {
  onSuccess?: () => void;
  onError?: () => void;
} = {};

jest.mock('@tanstack/react-query', () => {
  const original = jest.requireActual('@tanstack/react-query');
  return {
    ...original,
    useQuery: jest.fn(),
    useMutation: jest.fn((options) => {
      mutationOptions = options as typeof mutationOptions;
      return {
        mutate: (variables: unknown) => {
          mockMutate(variables);
        },
        error: null,
      };
    }),
  };
});

// Mock feature flags
jest.mock('@/common/feature_flags', () => {
  const original = jest.requireActual('@/common/feature_flags');
  return {
    ...original,
    useFeatureFlag: jest.fn(() => true),
  };
});

// Mock pRPC clients
jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useUfsClient: jest.fn(() => ({
    GetMachineLSE: {
      query: jest.fn(() => ({
        queryKey: ['mock-lse'],
        queryFn: jest.fn(),
      })),
    },
  })),
  useFleetConsoleClient: jest.fn(() => ({
    UpdateChromeOSDevice: jest.fn(),
  })),
}));

describe('<ChromeOSInventoryData />', () => {
  const mockDeviceOs = {
    id: 'test-host-name',
    deviceSpec: {
      labels: {},
    },
  } as Partial<Device> as Device;

  const mockDeviceOsPartner = {
    id: 'test-host-name-partner',
    deviceSpec: {
      labels: {
        ufs_namespace: {
          values: ['os-partner'],
        },
      },
    },
  } as Partial<Device> as Device;

  beforeEach(() => {
    jest.clearAllMocks();
    // Default mock behavior is loading state (returns undefined data)
    jest.mocked(useQuery).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as unknown as UseQueryResult<unknown, Error>);
  });

  it('renders CLI command for default namespace', async () => {
    render(
      <FakeContextProvider>
        <ChromeOSInventoryData device={mockDeviceOs} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('shivas get dut -json test-host-name'),
    ).toBeVisible();
  });

  it('renders CLI command for os-partner namespace', async () => {
    render(
      <FakeContextProvider>
        <ChromeOSInventoryData device={mockDeviceOsPartner} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        'shivas get dut -json -namespace os-partner test-host-name-partner',
      ),
    ).toBeVisible();
  });

  it('renders CLI command for labstation', async () => {
    jest.mocked(useQuery).mockReturnValue({
      data: {
        chromeosMachineLse: {
          deviceLse: {
            labstation: {
              hostname: 'test-host-name',
              pools: ['pool1'],
            },
          },
        },
      },
      isLoading: false,
      isError: false,
    } as unknown as UseQueryResult<unknown, Error>);

    render(
      <FakeContextProvider>
        <ChromeOSInventoryData device={mockDeviceOs} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('shivas get labstation -json test-host-name'),
    ).toBeVisible();
  });

  it('walks through the visual edit-save lifecycle successfully', async () => {
    const lseData = {
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            hostname: 'test-host-name',
            pools: ['pool1'],
            hive: 'hive-1',
          },
        },
      },
    };

    jest.mocked(useQuery).mockReturnValue({
      data: lseData,
      isLoading: false,
      isError: false,
    } as unknown as UseQueryResult<unknown, Error>);

    render(
      <FakeContextProvider>
        <ChromeOSInventoryData device={mockDeviceOs} />
      </FakeContextProvider>,
    );

    // 1. Initial State: Visual mode dashboard renders read-only values
    expect(screen.getByText('Visual Dashboard')).toBeInTheDocument();
    expect(screen.getByText('pool1')).toBeVisible();

    // 2. Click Edit on the Logical Scheduling card
    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    // 3. Update field: add new pool
    const input = screen.getByRole('textbox');
    expect(input).toHaveValue('pool1');
    await userEvent.clear(input);
    await userEvent.type(input, 'pool1,pool-new');
    expect(input).toHaveValue('pool1,pool-new');

    // Confirm card changes (closes card-level editing, stages updates globally)
    const confirmBtn = screen.getByRole('button', { name: /confirm/i });
    await userEvent.click(confirmBtn);

    // 4. Global Action Buttons appear
    const discardBtn = screen.getByRole('button', { name: /discard changes/i });
    const saveBtn = screen.getByRole('button', { name: /save all changes/i });
    expect(discardBtn).toBeVisible();
    expect(saveBtn).toBeVisible();

    // 5. Click Save All Changes -> Opens SaveDiffDialog
    await userEvent.click(saveBtn);

    const dialog = screen.getByRole('dialog');
    const withinDialog = within(dialog);

    expect(withinDialog.getByText('Review Changes')).toBeInTheDocument();
    expect(withinDialog.getByText('Pools')).toBeInTheDocument();
    expect(withinDialog.getByText('pool1')).toBeInTheDocument();
    expect(withinDialog.getByText('pool1,pool-new')).toBeInTheDocument();

    // 6. Click Confirm & Save -> Triggers mutation
    const confirmSaveBtn = withinDialog.getByRole('button', {
      name: /confirm & save/i,
    });

    mockMutate.mockImplementation(() => {
      // Simulate UFS mutation success
      mutationOptions.onSuccess?.();
    });

    await userEvent.click(confirmSaveBtn);

    // Verify mutation request arguments (including client request ID)
    expect(mockMutate).toHaveBeenCalledTimes(1);
    const mutationArg = mockMutate.mock.calls[0][0];
    expect(mutationArg.deviceId).toBe('test-host-name');
    expect(mutationArg.edits.pools).toEqual(['pool1', 'pool-new']);
    expect(mutationArg.updateMask).toEqual(['pools']);
    expect(mutationArg.requestId).toBeDefined(); // Client-side UUID should be present
    expect(typeof mutationArg.requestId).toBe('string');
    expect(mutationArg.requestId.length).toBeGreaterThan(0);

    // Verify success UI state shown and dialog closed
    expect(screen.getByText('Changes Saved Successfully')).toBeInTheDocument();
    const closeBtn = screen.getByRole('button', { name: /close/i });
    await userEvent.click(closeBtn);

    // After success, save state resets and changes buttons become hidden
    expect(discardBtn).not.toBeVisible();
    expect(saveBtn).not.toBeVisible();
  });
});
