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

import { BaselineChip } from './baseline_chip';

describe('BaselineChip', () => {
  it('renders Quota Baseline chip with correct tooltip on hover', async () => {
    render(
      <FakeContextProvider>
        <BaselineChip isAvailability={true} testId="test-quota-chip" />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('test-quota-chip');
    expect(chip).toHaveTextContent('Quota Baseline');
    fireEvent.mouseOver(chip);

    expect(
      await screen.findByText(
        /Availability is calculated as the percentage of READY devices against the model's expected quota target/i,
      ),
    ).toBeInTheDocument();
  });

  it('renders Enrolled Baseline chip with correct tooltip on hover', async () => {
    render(
      <FakeContextProvider>
        <BaselineChip isAvailability={false} testId="test-enrolled-chip" />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('test-enrolled-chip');
    expect(chip).toHaveTextContent('Enrolled Baseline');
    fireEvent.mouseOver(chip);

    expect(
      await screen.findByText(
        /Health is calculated as the percentage of READY devices out of actual enrolled physical DUTs/i,
      ),
    ).toBeInTheDocument();
  });
});
