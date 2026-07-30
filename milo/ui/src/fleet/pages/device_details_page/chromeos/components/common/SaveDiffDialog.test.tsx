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

import { render, screen, fireEvent } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FieldDiff } from '../../utils/inventory_editing_utils';

import { SaveDiffDialog } from './SaveDiffDialog';

describe('SaveDiffDialog', () => {
  const defaultProps = {
    open: true,
    saveState: 'review' as 'review' | 'saving' | 'success' | 'error',
    diffs: [
      { path: 'Pools', original: 'poolA', updated: 'poolB' },
    ] as FieldDiff[],
    shivasCommands: [
      'shivas update dut -name test-device -pools-replace poolB',
    ],
    deviceId: 'test-device',
    onConfirm: jest.fn(),
    onCancel: jest.fn(),
    onClose: jest.fn(),
    errorMessage: undefined as string | null | undefined,
  };

  const renderDialog = (
    props: React.ComponentProps<typeof SaveDiffDialog> = defaultProps,
  ) => {
    return render(
      <FakeContextProvider>
        <SaveDiffDialog {...props} />
      </FakeContextProvider>,
    );
  };

  beforeEach(() => {
    jest.clearAllMocks();
    Object.assign(navigator, {
      clipboard: {
        writeText: jest.fn().mockImplementation(() => Promise.resolve()),
      },
    });
  });

  it('renders review state with diff table and triggers confirm/cancel callbacks', () => {
    renderDialog();
    expect(screen.getByText('Review Changes')).toBeInTheDocument();
    expect(screen.getByText('Pools')).toBeInTheDocument();
    expect(screen.getByText('poolA')).toBeInTheDocument();
    expect(screen.getByText('poolB')).toBeInTheDocument();

    // Verify shivas command is rendered
    expect(
      screen.getByText(
        'shivas update dut -name test-device -pools-replace poolB',
      ),
    ).toBeInTheDocument();

    const saveButton = screen.getByRole('button', { name: /confirm & save/i });
    const cancelButton = screen.getByRole('button', { name: /cancel/i });

    fireEvent.click(saveButton);
    expect(defaultProps.onConfirm).toHaveBeenCalledTimes(1);

    fireEvent.click(cancelButton);
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it('renders saving state with circular progress', () => {
    renderDialog({ ...defaultProps, saveState: 'saving' });
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(
      screen.getByText(/saving changes to UFS service/i),
    ).toBeInTheDocument();
  });

  it('renders success state and triggers close callback', () => {
    renderDialog({ ...defaultProps, saveState: 'success' });
    expect(screen.getByText('Changes Saved Successfully')).toBeInTheDocument();

    // Verify changelog is rendered
    expect(screen.getByText(/Changelog/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Inventory updates for test-device/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/Pools.*poolA.*poolB/)).toBeInTheDocument();

    // Verify copy button works
    const copyButton = screen.getByRole('button', {
      name: /copy to clipboard/i,
    });
    fireEvent.click(copyButton);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      '**Inventory updates for test-device:**\n*   **Pools**: `poolA` ➔ `poolB`',
    );

    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });

  it('renders error state with Alert and triggers close callback', () => {
    renderDialog({
      ...defaultProps,
      saveState: 'error',
      errorMessage: 'UFS database write timeout',
    });
    expect(screen.getByText('Error Saving Changes')).toBeInTheDocument();
    expect(
      screen.getByText('Failed to write updates to UFS'),
    ).toBeInTheDocument();
    expect(screen.getByText('UFS database write timeout')).toBeInTheDocument();

    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });

  it('renders redeployment required warning alert when hasDeployableEdits is true', () => {
    renderDialog({
      ...defaultProps,
      hasDeployableEdits: true,
    });
    expect(screen.getByText(/redeployment required/i)).toBeInTheDocument();
    expect(
      screen.getByText(
        'Device will be locked and a deploy task will be run. No other tasks will be scheduled until deployment completes.',
      ),
    ).toBeInTheDocument();
  });

  it('renders deploy task scheduled info alert when deployTaskUrl is provided on success', () => {
    renderDialog({
      ...defaultProps,
      saveState: 'success',
      deployTaskUrl: 'https://ci.chromium.org/p/chromeos/builders/deploy/123',
      deployTaskStatus: { code: 0, message: '', details: [] },
    });
    expect(screen.getByText('Deploy Task Scheduled')).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /view deploy task/i }),
    ).toHaveAttribute(
      'href',
      'https://ci.chromium.org/p/chromeos/builders/deploy/123',
    );
  });

  it('renders deploy task scheduling failed warning alert when deployTaskStatus indicates error', () => {
    renderDialog({
      ...defaultProps,
      saveState: 'success',
      deployTaskUrl: 'https://ci.chromium.org/p/chromeos/builders/deploy/123',
      deployTaskStatus: {
        code: 2,
        message: 'Swarming service unavailable',
        details: [],
      },
    });
    expect(
      screen.getByText('Deploy Task Scheduling Failed'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/swarming service unavailable/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/please re-run the deploy task manually/i),
    ).toBeInTheDocument();
  });

  it('renders fallback redeployment required info alert on success when hasDeployableEdits is true and deployTaskUrl is omitted', () => {
    renderDialog({
      ...defaultProps,
      saveState: 'success',
      hasDeployableEdits: true,
    });
    expect(screen.getByText('Redeployment Required')).toBeInTheDocument();
    expect(
      screen.getByText(
        /a deploy task may be required to verify hardware changes/i,
      ),
    ).toBeInTheDocument();
  });
});
