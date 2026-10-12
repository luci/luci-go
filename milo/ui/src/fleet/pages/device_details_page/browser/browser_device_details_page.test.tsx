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

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BrowserDeviceDetailsPage } from './browser_device_details_page';
import { useBrowserDeviceData } from './use_browser_device_data';

jest.mock('./use_browser_device_data');
jest.mock('@/fleet/pages/device_details_page/common/bot_information', () => ({
  BotInformation: () => <div data-testid="bot-info" />,
}));
jest.mock('@/fleet/pages/device_details_page/common/bot_state', () => ({
  BotState: () => <div data-testid="bot-state" />,
}));

const mockUseBrowserDeviceData = useBrowserDeviceData as jest.Mock;

describe('<BrowserDeviceDetailsPage />', () => {
  afterEach(() => {
    localStorage.clear();
  });

  it('removes Bot info tab and redirects ?tab=bot-info to Dimensions', () => {
    mockUseBrowserDeviceData.mockReturnValue({
      isLoading: false,
      device: BrowserDevice.fromPartial({
        id: 'host-1',
        ufsLabels: { hostname: { values: ['host-1'] } },
        swarmingLabels: {},
      }),
    });

    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?tab=bot-info'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <BrowserDeviceDetailsPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(
      screen.queryByRole('tab', { name: 'Bot info' }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Dimensions' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });
});
