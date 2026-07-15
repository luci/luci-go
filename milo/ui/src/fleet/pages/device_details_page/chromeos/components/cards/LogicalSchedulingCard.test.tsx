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

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { LogicalSchedulingCard } from './LogicalSchedulingCard';

describe('<LogicalSchedulingCard />', () => {
  it('renders logical scheduling info and formatted logical zone correctly', async () => {
    render(
      <FakeContextProvider>
        <LogicalSchedulingCard pools={['cq', 'continuous']} logicalZone={1} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Pools & Task Routing')).toBeVisible();
    expect(screen.getByText('Pools')).toBeVisible();
    expect(screen.getByText('cq')).toBeVisible();
    expect(screen.getByText('continuous')).toBeVisible();
    expect(screen.getByText('Logical Zone')).toBeVisible();
    expect(screen.getByText('DRILLZONE_SFO36')).toBeVisible();
  });

  it('omits scheduling pools grid section when pools array is empty', async () => {
    render(
      <FakeContextProvider>
        <LogicalSchedulingCard logicalZone={1} pools={[]} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Logical Zone')).toBeVisible();
    expect(screen.queryByText('Pools')).not.toBeInTheDocument();
  });

  it('renders empty message when no logical scheduling telemetry exists', async () => {
    render(
      <FakeContextProvider>
        <LogicalSchedulingCard />
      </FakeContextProvider>,
    );
    expect(
      screen.getByText('No scheduling tags or pool labels assigned.'),
    ).toBeVisible();
  });

  it('renders edit button, handles isEditing state and onEdit click when editable is true', async () => {
    const handleEdit = jest.fn();
    const { rerender } = render(
      <FakeContextProvider>
        <LogicalSchedulingCard
          pools={['cq']}
          editable
          isEditing={false}
          onEdit={handleEdit}
        />
      </FakeContextProvider>,
    );

    const editBtn = screen.getByRole('button', {
      name: 'edit Pools & Task Routing',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(handleEdit).toHaveBeenCalledTimes(1);

    rerender(
      <FakeContextProvider>
        <LogicalSchedulingCard
          pools={['cq']}
          editable
          isEditing={true}
          onEdit={handleEdit}
        />
      </FakeContextProvider>,
    );
    expect(
      screen.getByRole('button', {
        name: 'edit Pools & Task Routing',
      }),
    ).toBeVisible();
  });

  it('omits Logical Zone when logicalZone is unspecified or 0', async () => {
    render(
      <FakeContextProvider>
        <LogicalSchedulingCard pools={['cq']} logicalZone={0} />
      </FakeContextProvider>,
    );

    expect(screen.queryByText('Logical Zone')).not.toBeInTheDocument();
  });
});
