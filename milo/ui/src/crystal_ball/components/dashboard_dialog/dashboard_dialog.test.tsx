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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { DashboardDialog } from '@/crystal_ball/components/dashboard_dialog';

describe('<DashboardDialog />', () => {
  const mockOnClose = jest.fn();
  const mockOnSubmit = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders create mode correctly', () => {
    render(
      <DashboardDialog
        open={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
      />,
    );

    expect(screen.getByText('Create New Dashboard')).toBeInTheDocument();
    expect(screen.getByLabelText(/Dashboard Name/i)).toHaveValue('');
    expect(screen.getByLabelText(/Description/i)).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  });

  it('renders edit mode correctly with initial data', () => {
    render(
      <DashboardDialog
        open={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        initialData={{
          name: 'dashboardStates/abcd123',
          displayName: 'Existing Dashboard',
          description: 'Existing Description',
          dashboardContent: {},
          updateTime: { seconds: 0, nanos: 0 },
          createTime: { seconds: 0, nanos: 0 },
          revisionId: '',
          etag: '',
          uid: '',
          reconciling: false,
        }}
      />,
    );

    expect(screen.getByText('Edit Dashboard Details')).toBeInTheDocument();
    expect(screen.getByLabelText(/Dashboard Name/i)).toHaveValue(
      'Existing Dashboard',
    );
    expect(screen.getByLabelText(/Description/i)).toHaveValue(
      'Existing Description',
    );
    expect(screen.getByRole('button', { name: 'Save' })).not.toBeDisabled();
  });

  it('submits form on click', async () => {
    mockOnSubmit.mockResolvedValueOnce(undefined);

    render(
      <DashboardDialog
        open={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
      />,
    );

    fireEvent.change(screen.getByLabelText(/Dashboard Name/i), {
      target: { value: 'New Name' },
    });
    fireEvent.change(screen.getByLabelText(/Description/i), {
      target: { value: 'New Desc' },
    });

    const saveButton = screen.getByRole('button', { name: 'Save' });
    expect(saveButton).not.toBeDisabled();
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        displayName: 'New Name',
        description: 'New Desc',
      });
    });
  });

  it('renders pending state', () => {
    render(
      <DashboardDialog
        open={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        initialData={{
          name: 'dashboardStates/abcd123',
          displayName: 'Existing Dashboard',
          description: 'Existing Description',
          dashboardContent: {},
          updateTime: { seconds: 0, nanos: 0 },
          createTime: { seconds: 0, nanos: 0 },
          revisionId: '',
          etag: '',
          uid: '',
          reconciling: false,
        }}
        isPending={true}
      />,
    );

    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  });

  it('renders error message', () => {
    render(
      <DashboardDialog
        open={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        errorMsg="Test Error Message"
      />,
    );

    expect(screen.getByText('Test Error Message')).toBeInTheDocument();
  });
});
