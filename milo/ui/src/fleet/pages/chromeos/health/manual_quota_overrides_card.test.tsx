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
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { ListModelQuotaOverridesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ManualQuotaOverridesCard } from './manual_quota_overrides_card';
import * as UseModelQuotaOverridesModule from './use_model_quota_overrides';

describe('ManualQuotaOverridesCard', () => {
  const mockSetMutateAsync = jest.fn();
  const mockDeleteMutateAsync = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  const setupMockHook = ({
    overrides = [
      { id: '1', model: 'brya', overriddenExpectedQuantity: 15 },
      { id: '2', model: 'volteer', overriddenExpectedQuantity: 20 },
    ],
    canEdit = true,
    isPending = false,
    isError = false,
    error = null as unknown,
    isPermissionLoading = false,
  } = {}) => {
    jest
      .spyOn(UseModelQuotaOverridesModule, 'useModelQuotaOverrides')
      .mockReturnValue({
        overridesQuery: {
          data: isError
            ? undefined
            : ({
                overrides,
              } as unknown as ListModelQuotaOverridesResponse),
          isPending,
          isError,
          error: error ?? (isError ? new Error('Failed to load') : null),
        } as unknown as UseQueryResult<ListModelQuotaOverridesResponse, Error>,
        setOverrideMutation: {
          mutateAsync: mockSetMutateAsync,
          isPending: false,
        } as unknown as ReturnType<
          typeof UseModelQuotaOverridesModule.useModelQuotaOverrides
        >['setOverrideMutation'],
        deleteOverrideMutation: {
          mutateAsync: mockDeleteMutateAsync,
          isPending: false,
        } as unknown as ReturnType<
          typeof UseModelQuotaOverridesModule.useModelQuotaOverrides
        >['deleteOverrideMutation'],
        canEdit,
        isPermissionLoading,
      });
  };

  it('renders card title, subtitle, temporary component note, and active overrides table', () => {
    setupMockHook();

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Manual Model Quota Overrides'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /Note: This is a temporary component that provides working functionality, but will be replaced with a performance ranking later on\./i,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('brya')).toBeInTheDocument();
    expect(screen.getByText('15')).toBeInTheDocument();
    expect(screen.getByText('volteer')).toBeInTheDocument();
    expect(screen.getByText('20')).toBeInTheDocument();
  });

  it('renders empty state when no overrides exist', () => {
    setupMockHook({ overrides: [] });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('No manual model quota overrides configured.'),
    ).toBeInTheDocument();
  });

  it('validates empty model and disables submission', () => {
    setupMockHook();

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const modelInput = screen.getByLabelText('Override Model Input');
    fireEvent.change(modelInput, { target: { value: '   ' } });

    expect(
      screen.getByText('Model identifier cannot be empty.'),
    ).toBeInTheDocument();
    const saveButton = screen.getByRole('button', {
      name: /set override/i,
    });
    expect(saveButton).toBeDisabled();
  });

  it('validates invalid quota (<= 0) and disables submission', () => {
    setupMockHook();

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const quotaInput = screen.getByLabelText('Override Quota Input');
    fireEvent.change(quotaInput, { target: { value: '0' } });

    expect(
      screen.getByText(
        'Expected quota must be a positive whole integer greater than 0.',
      ),
    ).toBeInTheDocument();
    const saveButton = screen.getByRole('button', {
      name: /set override/i,
    });
    expect(saveButton).toBeDisabled();
  });

  it('disables save button when model and quota match an existing override', () => {
    setupMockHook({
      overrides: [{ id: '1', model: 'brya', overriddenExpectedQuantity: 15 }],
    });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const modelInput = screen.getByLabelText('Override Model Input');
    const quotaInput = screen.getByLabelText('Override Quota Input');
    const saveButton = screen.getByRole('button', {
      name: /set override/i,
    });

    // Enter identical values (with whitespace and different casing)
    fireEvent.change(modelInput, { target: { value: '  BrYa  ' } });
    fireEvent.change(quotaInput, { target: { value: '15' } });

    expect(saveButton).toBeDisabled();

    // Changing quota to a different value enables submission
    fireEvent.change(quotaInput, { target: { value: '25' } });
    expect(saveButton).not.toBeDisabled();
  });

  it('allows lead users to set a new override successfully', async () => {
    setupMockHook();
    mockSetMutateAsync.mockResolvedValueOnce({
      override: { id: '3', model: 'dedede', overriddenExpectedQuantity: 30 },
    });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const modelInput = screen.getByLabelText('Override Model Input');
    const quotaInput = screen.getByLabelText('Override Quota Input');
    const saveButton = screen.getByRole('button', {
      name: /set override/i,
    });

    fireEvent.change(modelInput, { target: { value: 'dedede' } });
    fireEvent.change(quotaInput, { target: { value: '30' } });

    expect(saveButton).not.toBeDisabled();
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockSetMutateAsync).toHaveBeenCalledWith({
        model: 'dedede',
        overriddenExpectedQuantity: 30,
      });
    });

    expect(
      await screen.findByText(
        'Quota override for "dedede" saved successfully.',
      ),
    ).toBeInTheDocument();
  });

  it('displays error snackbar when saving override fails', async () => {
    setupMockHook();
    mockSetMutateAsync.mockRejectedValueOnce(new Error('Backend error'));

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const modelInput = screen.getByLabelText('Override Model Input');
    const quotaInput = screen.getByLabelText('Override Quota Input');
    const saveButton = screen.getByRole('button', {
      name: /set override/i,
    });

    fireEvent.change(modelInput, { target: { value: 'dedede' } });
    fireEvent.change(quotaInput, { target: { value: '30' } });

    fireEvent.click(saveButton);

    expect(await screen.findByText('Backend error')).toBeInTheDocument();
  });

  it('allows lead users to delete an override after confirming in dialog', async () => {
    setupMockHook();
    mockDeleteMutateAsync.mockResolvedValueOnce({});

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const deleteButtons = screen.getAllByLabelText(/delete override for/i);
    expect(deleteButtons).toHaveLength(2);

    // 1. Click delete icon on row
    fireEvent.click(deleteButtons[0]);

    // 2. Confirmation dialog should be rendered
    expect(screen.getByText('Remove Quota Override')).toBeInTheDocument();
    expect(
      screen.getByText(
        /Are you sure you want to remove the manual quota override for/i,
      ),
    ).toBeInTheDocument();

    // 3. Confirm deletion
    const confirmButton = screen.getByRole('button', { name: /^remove$/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockDeleteMutateAsync).toHaveBeenCalledWith('brya');
    });

    expect(
      await screen.findByText(
        'Quota override for "brya" removed successfully.',
      ),
    ).toBeInTheDocument();
  });

  it('allows lead users to cancel deleting an override', async () => {
    setupMockHook();

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    const deleteButtons = screen.getAllByLabelText(/delete override for/i);
    fireEvent.click(deleteButtons[0]);

    // Dialog is visible
    expect(screen.getByText('Remove Quota Override')).toBeInTheDocument();

    // Click Cancel
    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    // Dialog closes and delete mutation was not called
    await waitFor(() => {
      expect(
        screen.queryByText('Remove Quota Override'),
      ).not.toBeInTheDocument();
    });
    expect(mockDeleteMutateAsync).not.toHaveBeenCalled();
  });

  it('renders in read-only mode for non-lead users', () => {
    setupMockHook({ canEdit: false });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Read-only view (FLOPs Lead permission required).'),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Override Model Input')).toBeDisabled();
    expect(screen.getByLabelText('Override Quota Input')).toBeDisabled();
    expect(
      screen.queryByRole('button', { name: /set override/i }),
    ).not.toBeInTheDocument();

    // Delete buttons should not be present
    expect(
      screen.queryByLabelText(/delete override for/i),
    ).not.toBeInTheDocument();
  });

  it('displays loading spinner and hides form when query is pending', () => {
    setupMockHook({ isPending: true });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(screen.queryByLabelText('Override Model Input')).toBeNull();
    expect(screen.queryByLabelText('Override Quota Input')).toBeNull();
    expect(screen.queryByRole('button', { name: /set override/i })).toBeNull();
  });

  it('displays error alert and hides form when query fails', () => {
    setupMockHook({ isError: true });

    render(
      <FakeContextProvider>
        <ManualQuotaOverridesCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Failed to load model quota overrides.'),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Override Model Input')).toBeNull();
    expect(screen.queryByLabelText('Override Quota Input')).toBeNull();
    expect(screen.queryByRole('button', { name: /set override/i })).toBeNull();
  });
});
