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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RPMCard } from './RPMCard';

const renderWithProviders = (ui: React.ReactNode) =>
  render(<FakeContextProvider>{ui}</FakeContextProvider>);

describe('<RPMCard />', () => {
  it('renders DUT power RPM connection correctly', async () => {
    renderWithProviders(
      <RPMCard
        rpm={{
          powerunitName: 'chromeos15-row1-rpm',
          powerunitOutlet: 'AA1',
        }}
      />,
    );

    expect(screen.getByText('RPM')).toBeVisible();
    expect(screen.getByText('Name')).toBeVisible();
    expect(screen.getByText('Outlet')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-rpm')).toBeVisible();
    expect(screen.getByText('AA1')).toBeVisible();
  });

  it('hides empty fields when partial details exist', async () => {
    renderWithProviders(
      <RPMCard
        rpm={{
          powerunitName: 'chromeos15-row1-rpm',
        }}
      />,
    );

    expect(screen.getByText('RPM')).toBeVisible();
    expect(screen.getByText('Name')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-rpm')).toBeVisible();

    expect(screen.queryByText('Outlet')).toBeNull();
    expect(screen.queryByText('Type')).toBeNull();
    expect(screen.queryByText('N/A')).toBeNull();
  });

  it('renders type unspecified or 0 correctly', async () => {
    renderWithProviders(
      <RPMCard
        rpm={{
          powerunitName: 'chromeos15-row1-rpm',
          powerunitType: 0,
        }}
      />,
    );

    expect(screen.getByText('RPM')).toBeVisible();
    expect(screen.getByText('Name')).toBeVisible();
    expect(screen.getByText('Type')).toBeVisible();
    expect(screen.getByText('UNKNOWN')).toBeVisible();
  });

  it('renders empty message when no RPM telemetry exists', () => {
    renderWithProviders(<RPMCard />);
    expect(
      screen.getByText(
        'No RPM outlets or power distribution units configured.',
      ),
    ).toBeInTheDocument();
  });
});
