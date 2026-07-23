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

import { FieldDiff } from '../../utils/inventory_editing_utils';

import { SaveDiffDialog } from './SaveDiffDialog';

describe('SaveDiffDialog', () => {
  const defaultProps = {
    open: true,
    saveState: 'review' as const,
    diffs: [
      { path: 'Pools', original: 'poolA', updated: 'poolB' },
    ] as FieldDiff[],
    onConfirm: jest.fn(),
    onCancel: jest.fn(),
    onClose: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders review state with diff table and triggers confirm/cancel callbacks', () => {
    render(<SaveDiffDialog {...defaultProps} />);
    expect(screen.getByText('Review Changes')).toBeInTheDocument();
    expect(screen.getByText('Pools')).toBeInTheDocument();
    expect(screen.getByText('poolA')).toBeInTheDocument();
    expect(screen.getByText('poolB')).toBeInTheDocument();

    const saveButton = screen.getByRole('button', { name: /confirm & save/i });
    const cancelButton = screen.getByRole('button', { name: /cancel/i });

    fireEvent.click(saveButton);
    expect(defaultProps.onConfirm).toHaveBeenCalledTimes(1);

    fireEvent.click(cancelButton);
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it('renders saving state with circular progress', () => {
    render(<SaveDiffDialog {...defaultProps} saveState="saving" />);
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(
      screen.getByText(/saving changes to UFS service/i),
    ).toBeInTheDocument();
  });

  it('renders success state and triggers close callback', () => {
    render(<SaveDiffDialog {...defaultProps} saveState="success" />);
    expect(screen.getByText('Changes Saved Successfully')).toBeInTheDocument();

    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });

  it('renders error state with Alert and triggers close callback', () => {
    render(
      <SaveDiffDialog
        {...defaultProps}
        saveState="error"
        errorMessage="UFS database write timeout"
      />,
    );
    expect(screen.getByText('Error Saving Changes')).toBeInTheDocument();
    expect(
      screen.getByText('Failed to write updates to UFS'),
    ).toBeInTheDocument();
    expect(screen.getByText('UFS database write timeout')).toBeInTheDocument();

    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });
});
