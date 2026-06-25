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

import { act, fireEvent, render, screen } from '@testing-library/react';

import { SettingsProvider } from '@/fleet/context/providers';
import { FC_CellProps } from '@/fleet/types/table';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDevice } from './chromeos_types';
import { DutStateCell } from './dut_state_cell';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({
    trackEvent: mockTrackEvent,
  }),
}));

describe('DutStateCell', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });
  test('should render nothing when the state is empty', () => {
    const mockProps = {
      cell: {
        getValue: () => '',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {},
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    const { container } = render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(container).toBeEmptyDOMElement();
  });

  test('should render only the chip when the state is not RESERVED', () => {
    const mockProps = {
      cell: {
        getValue: () => 'READY',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {
              dut_state: { values: ['READY'] },
            },
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Should render the chip
    expect(screen.getByText('READY')).toBeInTheDocument();

    // Should NOT render the info tooltip trigger
    expect(
      screen.queryByLabelText('show reservation details'),
    ).not.toBeInTheDocument();
  });

  test('should render the chip and info icon when the state is RESERVED', () => {
    const mockProps = {
      cell: {
        getValue: () => 'RESERVED',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {
              dut_state: { values: ['RESERVED'] },
              'ufs.dut_state_reason': { values: ['test comment'] },
            },
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Should render the chip
    expect(screen.getByText('RESERVED')).toBeInTheDocument();

    // Should render the info tooltip trigger
    expect(
      screen.getByLabelText('show reservation details'),
    ).toBeInTheDocument();
  });

  test('should display tooltip on hover with the correct reserve comment', async () => {
    const mockProps = {
      cell: {
        getValue: () => 'RESERVED',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {
              dut_state: { values: ['RESERVED'] },
              'ufs.dut_state_reason': {
                values: ['This device is reserved for manual testing'],
              },
            },
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const infoTrigger = screen.getByLabelText('show reservation details');

    // Tooltip should not be in the document initially
    expect(
      screen.queryByText(/DUT is currently reserved for:/),
    ).not.toBeInTheDocument();

    // Trigger mouseOver to ensure tooltip opens, logging the hover event, and wait for it
    fireEvent.mouseOver(infoTrigger);
    await screen.findByText('DUT is currently reserved for:');

    // Should track the GA event on hover
    expect(mockTrackEvent).toHaveBeenCalledWith('reserve_info_hovered', {
      componentName: 'reserve_info_button',
    });

    // Tooltip should now be open and display the correct comment
    expect(
      screen.getByText('DUT is currently reserved for:'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('This device is reserved for manual testing'),
    ).toBeInTheDocument();
  });

  test('should display tooltip on focus and track GA event for keyboard accessibility', async () => {
    const mockProps = {
      cell: {
        getValue: () => 'RESERVED',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {
              dut_state: { values: ['RESERVED'] },
              'ufs.dut_state_reason': {
                values: ['This device is reserved for manual testing'],
              },
            },
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const infoTrigger = screen.getByLabelText('show reservation details');

    // Tooltip should not be in the document initially
    expect(
      screen.queryByText(/DUT is currently reserved for:/),
    ).not.toBeInTheDocument();

    // Trigger focus natively inside act to ensure focus event is logged and avoid warnings
    act(() => {
      infoTrigger.focus();
    });

    // Should track the GA event on focus
    expect(mockTrackEvent).toHaveBeenCalledWith('reserve_info_hovered', {
      componentName: 'reserve_info_button',
    });
  });

  test('should display fallback text in tooltip if comment is missing', async () => {
    const mockProps = {
      cell: {
        getValue: () => 'RESERVED',
      },
      row: {
        original: {
          id: 'device-1',
          deviceSpec: {
            labels: {
              dut_state: { values: ['RESERVED'] },
              // 'ufs.dut_state_reason' is missing!
            },
          },
        },
      },
    } as unknown as FC_CellProps<ChromeOSDevice>;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell params={mockProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const infoTrigger = screen.getByLabelText('show reservation details');

    // Trigger mouseOver to ensure tooltip opens, logging the hover event, and wait for it
    fireEvent.mouseOver(infoTrigger);
    await screen.findByText('DUT is currently reserved for:');

    // Should track the GA event on hover
    expect(mockTrackEvent).toHaveBeenCalledWith('reserve_info_hovered', {
      componentName: 'reserve_info_button',
    });

    // Tooltip should display the fallback text
    expect(
      screen.getByText('DUT is currently reserved for:'),
    ).toBeInTheDocument();
    expect(screen.getByText('No comment provided')).toBeInTheDocument();
  });
});
