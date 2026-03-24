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

import '@testing-library/jest-dom';

import { render, screen, fireEvent } from '@testing-library/react';

import { DashboardState } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ShareDashboardDialog } from './share_dashboard_dialog';

Object.assign(navigator, {
  clipboard: {
    writeText: jest.fn(),
  },
});

const defaultDashboardState = DashboardState.fromPartial({
  name: 'dashboardStates/123',
  isPublic: false,
});

const defaultProps = {
  open: true,
  onClose: jest.fn(),
  onApplyPermissions: jest.fn(),
  dashboardState: defaultDashboardState,
};

describe('ShareDashboardDialog', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders closed when open is false', () => {
    render(<ShareDashboardDialog {...defaultProps} open={false} />);
    expect(screen.queryByText('Share Dashboard')).not.toBeInTheDocument();
  });

  it('renders with correct elements', () => {
    render(<ShareDashboardDialog {...defaultProps} />);
    expect(screen.getByText('Share Dashboard')).toBeInTheDocument();
    expect(screen.getByLabelText(/Public Access/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Apply Changes' }),
    ).toBeInTheDocument();
  });

  it('copies link to clipboard when copy button is clicked', () => {
    render(<ShareDashboardDialog {...defaultProps} />);
    const copyButton = screen.getByRole('button', { name: /Copy/i });
    fireEvent.click(copyButton);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      window.location.href,
    );
  });

  it('calls onApplyPermissions with draft state', () => {
    render(<ShareDashboardDialog {...defaultProps} />);
    const switchElement = screen.getByRole('checkbox', {
      name: /Public Access/i,
    });

    // Initial state
    expect(switchElement).not.toBeChecked();

    // Toggle switch
    fireEvent.click(switchElement);
    expect(switchElement).toBeChecked();

    // Apply
    const applyButton = screen.getByRole('button', { name: 'Apply Changes' });
    expect(applyButton).not.toBeDisabled();
    fireEvent.click(applyButton);

    expect(defaultProps.onApplyPermissions).toHaveBeenCalledWith(true);
  });

  it('initializes draftIsPublic from dashboardState', () => {
    const publicDashboardState = DashboardState.fromPartial({
      name: 'dashboardStates/123',
      isPublic: true,
    });
    render(
      <ShareDashboardDialog
        {...defaultProps}
        dashboardState={publicDashboardState}
      />,
    );
    const switchElement = screen.getByRole('checkbox', {
      name: /Public Access/i,
    });
    expect(switchElement).toBeChecked();

    // Toggle
    fireEvent.click(switchElement);
    expect(switchElement).not.toBeChecked();

    // Apply
    const applyButton = screen.getByRole('button', { name: 'Apply Changes' });
    expect(applyButton).not.toBeDisabled();
    fireEvent.click(applyButton);

    expect(defaultProps.onApplyPermissions).toHaveBeenCalledWith(false);
  });

  it('disables buttons when isPending is true', () => {
    render(<ShareDashboardDialog {...defaultProps} isPending={true} />);
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled();
  });
});
