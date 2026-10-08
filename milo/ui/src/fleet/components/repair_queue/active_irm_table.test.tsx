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

import {
  ActiveIrmTable,
  ActiveIrmTableProps,
  RepairIrmIncident,
} from './active_irm_table';

const MOCK_INCIDENTS: readonly RepairIrmIncident[] = [
  {
    id: '1',
    masterBugId: '  12345678  ',
    title: 'Fleet wide lab issue',
  },
  { id: '2', masterBugId: '', title: '' },
];

describe('<ActiveIrmTable />', () => {
  const renderComponent = (overrides: Partial<ActiveIrmTableProps> = {}) =>
    render(
      <FakeContextProvider>
        <ActiveIrmTable
          incidents={[]}
          isLoading={false}
          isError={false}
          error={null}
          {...overrides}
        />
      </FakeContextProvider>,
    );

  it('renders loading state', () => {
    renderComponent({ isLoading: true });

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(
      screen.queryByTestId('active-irm-table-container'),
    ).not.toBeInTheDocument();
  });

  it('renders error state', () => {
    renderComponent({
      isError: true,
      error: new Error('Failed to load IRM incidents'),
    });

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(
      screen.getByText(/Failed to load IRM incidents/i),
    ).toBeInTheDocument();
  });

  it('renders empty state', () => {
    renderComponent();

    expect(screen.getByText('No active IRM incidents')).toBeInTheDocument();
  });

  it('renders incidents with bug links and fallbacks', () => {
    renderComponent({ incidents: MOCK_INCIDENTS });

    const link = screen.getByRole('link', { name: /b\/12345678/i });
    expect(link).toHaveAttribute(
      'href',
      'https://b.corp.google.com/issues/12345678',
    );
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');

    expect(screen.getByText('Fleet wide lab issue')).toBeInTheDocument();
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });
});
