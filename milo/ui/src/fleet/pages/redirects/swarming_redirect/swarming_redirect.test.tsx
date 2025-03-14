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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { SwarmingRedirect } from '.';

const FakeDeviceDetails = () => {
  return <>Fake Device details</>;
};

const FakeDevices = () => {
  return <>Fake Devices</>;
};

const FakePage = ({ path }: { path: string }) => (
  <FakeContextProvider
    mountedPath={path}
    routerOptions={{
      initialEntries: [path],
    }}
    siblingRoutes={[
      {
        path: '/ui/fleet/redirects/swarming/*',
        element: <SwarmingRedirect />,
      },

      {
        path: '/ui/fleet/labs/devices/:id',
        element: <FakeDeviceDetails />,
      },

      {
        path: '/ui/fleet/labs/devices',
        element: <FakeDevices />,
      },
    ]}
  >
    <SwarmingRedirect />
  </FakeContextProvider>
);

jest.mock('@/swarming/hooks/prpc_clients', () => ({
  useBotsClient: () => ({
    GetBot: () => ({
      dimensions: [{ key: 'dut_name', value: 'dut_name_value' }],
    }),
  }),
}));

const mockUseParams = jest.fn().mockReturnValue({});
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useParams: () => mockUseParams(),
}));

describe('<SwarmingRedirect />', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });
  it('redirects to /devices', async () => {
    // Create a fake device detail page to test route navigation.
    mockUseParams.mockReturnValue({ '*': 'botlist' });
    render(<FakePage path="/ui/fleet/redirects/swarming/botlist" />);

    await screen.findByText('Fake Devices');
    expect(screen.getByText('Fake Devices')).toBeInTheDocument();
  });

  it('redirects to /devices/:id', async () => {
    // Create a fake device detail page to test route navigation.
    mockUseParams.mockReturnValue({ '*': 'bot' });
    render(<FakePage path="/ui/fleet/redirects/swarming/bot?id=123" />);

    await screen.findByText('Fake Device details');
    expect(screen.getByText('Fake Device details')).toBeInTheDocument();
  });
});
