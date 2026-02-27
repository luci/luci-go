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

import { DashboardState } from '@/crystal_ball/types';

import { DeleteDashboardDialog } from './delete_dashboard_dialog';

describe('<DeleteDashboardDialog />', () => {
  const mockOnClose = jest.fn();
  const mockOnConfirm = jest.fn();
  const sampleDashboardState: DashboardState = {
    name: 'dashboardStates/123',
    displayName: 'Test Dashboard',
    dashboardContent: {},
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders correctly when open', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={sampleDashboardState}
      />,
    );

    expect(screen.getByText('Delete Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Test Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByText('Delete')).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <DeleteDashboardDialog
        open={false}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={sampleDashboardState}
      />,
    );

    expect(screen.queryByText('Delete Dashboard')).not.toBeInTheDocument();
  });

  it('calls onClose when Cancel is clicked', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={sampleDashboardState}
      />,
    );

    fireEvent.click(screen.getByText('Cancel'));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('calls onConfirm when Delete is clicked', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={sampleDashboardState}
      />,
    );

    fireEvent.click(screen.getByText('Delete'));
    expect(mockOnConfirm).toHaveBeenCalledTimes(1);
  });

  it('disables buttons and changes text when isDeleting is true', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={true}
        dashboardState={sampleDashboardState}
      />,
    );

    const cancelButton = screen.getByText('Cancel');
    const deleteButton = screen.getByText('Deleting...');

    expect(cancelButton).toBeDisabled();
    expect(deleteButton).toBeDisabled();
  });

  it('uses name if displayName is missing', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={{
          name: 'dashboardStates/abcd',
          dashboardContent: {},
        }}
      />,
    );

    expect(screen.getByText('dashboardStates/abcd')).toBeInTheDocument();
  });

  it('uses fallback text if both name and displayName are missing', () => {
    render(
      <DeleteDashboardDialog
        open={true}
        onClose={mockOnClose}
        onConfirm={mockOnConfirm}
        isDeleting={false}
        dashboardState={null}
      />,
    );

    expect(screen.getByText('this dashboard')).toBeInTheDocument();
  });
});
