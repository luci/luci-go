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

import { fireEvent, render, screen } from '@testing-library/react';

import { AssigneeCell, AssigneeCellProps } from './assignee_cell';

describe('<AssigneeCell />', () => {
  const claimTask = jest.fn();
  const unclaimTask = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  const renderCell = (overrides: Partial<AssigneeCellProps> = {}) =>
    render(
      <AssigneeCell
        taskId="101"
        currentUser="me@google.com"
        isPending={false}
        claimTask={claimTask}
        unclaimTask={unclaimTask}
        {...overrides}
      />,
    );

  it('renders a Claim button for unclaimed tasks', () => {
    renderCell({ claimedBy: '  ' });

    fireEvent.click(screen.getByRole('button', { name: 'Claim' }));
    expect(claimTask).toHaveBeenCalledWith({ taskId: '101' });
    expect(unclaimTask).not.toHaveBeenCalled();
  });

  it('disables the Claim button while a mutation is pending', () => {
    renderCell({ isPending: true });

    expect(screen.getByRole('button', { name: 'Claim' })).toBeDisabled();
  });

  it('unclaims when the current user clicks their own avatar', async () => {
    renderCell({ claimedBy: 'user:me@google.com' });

    const avatar = screen.getByText('M');
    fireEvent.mouseOver(avatar);
    expect(
      await screen.findByText('Assigned to you (click to unclaim)'),
    ).toBeInTheDocument();

    fireEvent.click(avatar);
    expect(unclaimTask).toHaveBeenCalledWith({ taskId: '101' });
    expect(claimTask).not.toHaveBeenCalled();
  });

  it("claims when clicking another technician's avatar", async () => {
    renderCell({ claimedBy: 'other@google.com' });

    const avatar = screen.getByText('O');
    fireEvent.mouseOver(avatar);
    expect(
      await screen.findByText(
        'Assigned to other@google.com (click to assign to yourself)',
      ),
    ).toBeInTheDocument();

    fireEvent.click(avatar);
    expect(claimTask).toHaveBeenCalledWith({ taskId: '101' });
    expect(unclaimTask).not.toHaveBeenCalled();
  });
});
