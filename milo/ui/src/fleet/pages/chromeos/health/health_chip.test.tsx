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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HealthChip } from './health_chip';

describe('HealthChip', () => {
  it('renders chip label with default styling', () => {
    render(
      <FakeContextProvider>
        <HealthChip label="Last 72h" testId="test-health-chip" />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('test-health-chip');
    expect(chip).toHaveTextContent('Last 72h');
  });

  it('renders chip with tooltip on hover', async () => {
    render(
      <FakeContextProvider>
        <HealthChip
          label="Last 72h"
          testId="test-health-chip"
          tooltip="Rolling 72 hour time window"
        />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('test-health-chip');
    fireEvent.mouseOver(chip);

    expect(
      await screen.findByText('Rolling 72 hour time window'),
    ).toBeInTheDocument();
  });

  it('supports primary color variant', () => {
    render(
      <FakeContextProvider>
        <HealthChip
          label="Active Filter"
          color="primary"
          testId="test-primary-chip"
        />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('test-primary-chip');
    expect(chip).toHaveTextContent('Active Filter');
  });
});
