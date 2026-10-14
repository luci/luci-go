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
import { fireEvent, render, screen } from '@testing-library/react';
import { useLocation } from 'react-router';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidDeviceIdCell } from './android_device_id_cell';

const NavigatedFromLink = () => {
  const { state } = useLocation();
  return <div data-testid="navigated-from">{state?.navigatedFromLink}</div>;
};

describe('AndroidDeviceIdCell', () => {
  it('passes the current page as navigatedFromLink to the device details page', () => {
    window.history.pushState({}, '', '/ui/fleet/p/pixel/repairs?page=2');

    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/p/pixel/repairs"
        siblingRoutes={[
          {
            path: '/ui/fleet/p/pixel/devices/:id',
            element: <NavigatedFromLink />,
          },
        ]}
        routerOptions={{ initialEntries: ['/ui/fleet/p/pixel/repairs?page=2'] }}
      >
        <AndroidDeviceIdCell deviceId="pixel-1" workspace="Pixel" />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('link', { name: 'pixel-1' }));

    expect(screen.getByTestId('navigated-from')).toHaveTextContent(
      '/ui/fleet/p/pixel/repairs?page=2',
    );
  });
});
