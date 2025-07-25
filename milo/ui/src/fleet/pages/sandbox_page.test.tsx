// Copyright 2025 The LUCI Authors.
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
import fetchMock from 'fetch-mock-jest';

import { useUfsClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { SandboxPage } from './sandbox_page';

jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useUfsClient: jest.fn(),
}));

const mockedUseUfsClient = useUfsClient as jest.Mock;

describe('<SandboxPage />', () => {
  beforeEach(() => {
    mockFetchAuthState();
    mockedUseUfsClient.mockReturnValue({
      ListMachines: {
        query: jest.fn().mockReturnValue({
          queryKey: ['ListMachines'],
          queryFn: () => Promise.resolve({ machines: [{ name: 'machine-1' }] }),
        }),
      },
    });
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
    jest.clearAllMocks();
  });

  it('should render', async () => {
    render(
      <FakeContextProvider>
        <SandboxPage />
      </FakeContextProvider>,
    );

    expect(screen.getByText(/This is a sandbox page/)).toBeVisible();
    expect(await screen.findByText(/"name": "machine-1"/)).toBeVisible();
  });
});
