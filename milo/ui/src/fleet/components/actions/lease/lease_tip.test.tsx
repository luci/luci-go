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
import { act } from 'react';

import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { LeaseTip } from './lease_tip';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

describe('<LeaseTip />', () => {
  it('renders CLI command for leasing device', async () => {
    render(
      <FakeAuthStateProvider>
        <LeaseTip hostname="test-device" />
      </FakeAuthStateProvider>,
    );

    // Open the dialog.
    await act(() => {
      const button = screen.getByRole('button');
      button.click();
    });

    expect(mockTrackEvent).toHaveBeenCalledWith('lease_tip', {
      componentName: 'lease_tip_button',
    });

    expect(
      screen.getByText('crosfleet dut lease -host test-device'),
    ).toBeVisible();
  });
});
