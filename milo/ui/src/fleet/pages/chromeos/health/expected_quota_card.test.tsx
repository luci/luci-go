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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { UseQueryResult } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { GetDefaultQuotaResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ExpectedQuotaCard } from './expected_quota_card';
import * as UseDefaultQuotaModule from './use_default_quota';

describe('ExpectedQuotaCard', () => {
  const mockMutateAsync = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  const setupMockHook = ({
    defaultQuota = 49 as number | undefined,
    canEdit = true,
    isPending = false,
    isError = false,
    error = null as unknown,
    isPermissionLoading = false,
  } = {}) => {
    jest.spyOn(UseDefaultQuotaModule, 'useDefaultQuota').mockReturnValue({
      quotaQuery: {
        data:
          isError || defaultQuota === undefined
            ? undefined
            : ({
                defaultQuota,
              } as unknown as GetDefaultQuotaResponse),
        isPending,
        isError,
        error: error ?? (isError ? new Error('Failed to load') : null),
      } as unknown as UseQueryResult<GetDefaultQuotaResponse, Error>,
      setQuotaMutation: {
        mutateAsync: mockMutateAsync,
        isPending: false,
      } as unknown as ReturnType<
        typeof UseDefaultQuotaModule.useDefaultQuota
      >['setQuotaMutation'],
      canEdit,
      isPermissionLoading,
    });
  };

  it('renders card title and current quota value', () => {
    setupMockHook({ defaultQuota: 49 });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Global Default Expected Quota'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Global default quota defines the expected fleet count/),
    ).toBeInTheDocument();
    const input = screen.getByLabelText('Default Quota Input');
    expect(input).toHaveValue(49);
  });

  it('renders in read-only mode for non-lead users', () => {
    setupMockHook({ defaultQuota: 49, canEdit: false });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    const input = screen.getByLabelText('Default Quota Input');
    expect(input).toBeDisabled();
    expect(
      screen.getByText(
        'Read-only view (FLOPs Lead permission required to modify).',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /save/i })).toBeNull();
  });

  it('does not display read-only helper text when permissions are still loading', () => {
    setupMockHook({
      defaultQuota: 49,
      canEdit: false,
      isPermissionLoading: true,
    });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    expect(
      screen.queryByText(
        'Read-only view (FLOPs Lead permission required to modify).',
      ),
    ).toBeNull();
  });

  it('allows lead users to modify quota and save successfully', async () => {
    mockMutateAsync.mockResolvedValueOnce({
      defaultQuota: 60,
    });
    setupMockHook({ defaultQuota: 49, canEdit: true });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    const input = screen.getByLabelText('Default Quota Input');
    const saveButton = screen.getByRole('button', { name: /save/i });

    // Initially save is disabled because value is unchanged
    expect(saveButton).toBeDisabled();

    // Change input value to 60
    fireEvent.change(input, { target: { value: '60' } });
    expect(saveButton).toBeEnabled();

    // Click Save
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(60);
    });

    expect(
      await screen.findByText('Default quota updated successfully.'),
    ).toBeInTheDocument();
  });

  it('submits on Enter key press inside input', async () => {
    mockMutateAsync.mockResolvedValueOnce({
      defaultQuota: 65,
    });
    setupMockHook({ defaultQuota: 49, canEdit: true });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    const input = screen.getByLabelText('Default Quota Input');
    fireEvent.change(input, { target: { value: '65' } });
    fireEvent.submit(input);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(65);
    });

    expect(
      await screen.findByText('Default quota updated successfully.'),
    ).toBeInTheDocument();
  });

  it('displays error and disables save when invalid quota (<= 0) is entered', () => {
    setupMockHook({ defaultQuota: 49, canEdit: true });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    const input = screen.getByLabelText('Default Quota Input');
    const saveButton = screen.getByRole('button', { name: /save/i });

    // Enter 0
    fireEvent.change(input, { target: { value: '0' } });
    expect(
      screen.getByText(
        'Expected quota must be a positive whole integer greater than 0.',
      ),
    ).toBeInTheDocument();
    expect(saveButton).toBeDisabled();

    // Enter negative number
    fireEvent.change(input, { target: { value: '-10' } });
    expect(
      screen.getByText(
        'Expected quota must be a positive whole integer greater than 0.',
      ),
    ).toBeInTheDocument();
    expect(saveButton).toBeDisabled();

    // Enter decimal number
    fireEvent.change(input, { target: { value: '12.5' } });
    expect(
      screen.getByText(
        'Expected quota must be a positive whole integer greater than 0.',
      ),
    ).toBeInTheDocument();
    expect(saveButton).toBeDisabled();

    // Enter number exceeding int32 max
    fireEvent.change(input, { target: { value: '3000000000' } });
    expect(
      screen.getByText('Expected quota cannot exceed 2,147,483,647.'),
    ).toBeInTheDocument();
    expect(saveButton).toBeDisabled();
  });

  it('displays error alert on save failure', async () => {
    mockMutateAsync.mockRejectedValueOnce(
      new Error('Permission denied: not a lead'),
    );
    setupMockHook({ defaultQuota: 49, canEdit: true });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    const input = screen.getByLabelText('Default Quota Input');
    const saveButton = screen.getByRole('button', { name: /save/i });

    fireEvent.change(input, { target: { value: '75' } });
    fireEvent.click(saveButton);

    expect(
      await screen.findByText('Permission denied: not a lead'),
    ).toBeInTheDocument();
  });

  it('displays error alert when initial query fails with a generic error', () => {
    setupMockHook({
      isError: true,
      error: new Error('Internal server error'),
    });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Failed to load default quota.'),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Default Quota Input')).toBeNull();
  });

  it('allows lead user to configure quota when default quota is not found on backend (NOT_FOUND)', async () => {
    mockMutateAsync.mockResolvedValueOnce({
      defaultQuota: 50,
    });
    setupMockHook({
      defaultQuota: undefined,
      isError: true,
      error: new GrpcError(RpcCode.NOT_FOUND, 'not found'),
      canEdit: true,
    });

    render(
      <FakeContextProvider>
        <ExpectedQuotaCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        'No default quota is currently configured. Enter a value below to set it.',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText('Failed to load default quota.')).toBeNull();

    const input = screen.getByLabelText('Default Quota Input');
    expect(input).toHaveValue(null);

    const saveButton = screen.getByRole('button', { name: /save/i });
    expect(saveButton).toBeDisabled();

    fireEvent.change(input, { target: { value: '50' } });
    expect(saveButton).toBeEnabled();

    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(50);
    });

    expect(
      await screen.findByText('Default quota updated successfully.'),
    ).toBeInTheDocument();
  });
});
