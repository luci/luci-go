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
import { useParams } from 'react-router';

import {
  MOCK_DEVICE_1,
  MOCK_DEVICE_2,
  mockErrorListingDevices,
  mockListDevices,
} from '@/fleet/testing_tools/mocks/devices_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { SingleDeviceRedirect } from './single_device_redirect';

const FakeDeviceDetails = () => {
  const { id } = useParams();
  return <>Fake Device details: {id}</>;
};

describe('<SingleDeviceRedirect />', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('redirects to first device in search', async () => {
    mockListDevices([MOCK_DEVICE_1, MOCK_DEVICE_2], '');

    // Create a fake device detail page to test route navigation.
    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/redirects/singledevice"
        routerOptions={{
          initialEntries: ['/ui/fleet/redirects/singledevice'],
        }}
        siblingRoutes={[
          {
            path: '/ui/fleet/labs/devices/:id',
            element: <FakeDeviceDetails />,
          },
        ]}
      >
        <SingleDeviceRedirect />
      </FakeContextProvider>,
    );

    await screen.findByText('Fake Device details: test-device-1');

    expect(
      screen.getByText('Fake Device details: test-device-1'),
    ).toBeVisible();
  });

  it('warns when no devices match', async () => {
    mockListDevices([], '');

    render(
      <FakeContextProvider>
        <SingleDeviceRedirect />
      </FakeContextProvider>,
    );

    await screen.findByTestId('single-device-redirect');

    expect(screen.getByText(/No devices matched the search/)).toBeVisible();
  });

  it('warns when error', async () => {
    mockErrorListingDevices();

    render(
      <FakeContextProvider>
        <SingleDeviceRedirect />
      </FakeContextProvider>,
    );

    await screen.findByTestId('single-device-redirect');

    expect(screen.getByText(/Redirection failed/)).toBeVisible();
  });
});
