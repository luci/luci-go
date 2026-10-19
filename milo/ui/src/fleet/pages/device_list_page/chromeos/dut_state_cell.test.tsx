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
import { colors } from '@/fleet/theme/colors';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  dutState,
  getServoStateColor,
  getServoUsbStateColor,
  getStatusColor,
} from './dut_state';
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
    const { container } = render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell state="" />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(container).toBeEmptyDOMElement();
  });

  test('should render only the chip when the state is not RESERVED', () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell state="READY" />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Should render the chip
    expect(screen.getByText('Ready')).toBeInTheDocument();

    // Should NOT render the info tooltip trigger
    expect(
      screen.queryByLabelText('show reservation details'),
    ).not.toBeInTheDocument();
  });

  test('should render the chip and info icon when the state is RESERVED', () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell state="RESERVED" comment="test comment" />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Should render the chip
    expect(screen.getByText('Reserved')).toBeInTheDocument();

    // Should render the info tooltip trigger
    expect(
      screen.getByLabelText('show reservation details'),
    ).toBeInTheDocument();
  });

  test('should display tooltip on hover with the correct reserve comment', async () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell
            state="RESERVED"
            comment="This device is reserved for manual testing"
          />
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
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell
            state="RESERVED"
            comment="This device is reserved for manual testing"
          />
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
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell state="RESERVED" />
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

  test('should format multi-word states into sentence case', () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <DutStateCell state="NEEDS_MANUAL_REPAIR" />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Needs manual repair')).toBeInTheDocument();
  });

  test('should use orange for automated REPAIR_FAILED and red for NEEDS_MANUAL_REPAIR', () => {
    expect(getStatusColor(dutState.REPAIR_FAILED)).toBe(colors.orange[100]);
    expect(getStatusColor(dutState.NEEDS_REPAIR)).toBe(colors.orange[100]);
    expect(getStatusColor(dutState.NEEDS_MANUAL_REPAIR)).toBe(colors.red[100]);
  });

  test('should map Servo State values to semantic chip colors across healthy, neutral, and broken states', () => {
    expect(getServoStateColor('WORKING')).toBe(colors.green[100]);
    expect(getServoStateColor('working')).toBe(colors.green[100]);
    expect(getServoStateColor('OK')).toBe(colors.green[100]);
    expect(getServoStateColor('BROKEN')).toBe(colors.red[100]);
    expect(getServoStateColor('NEED_REPLACEMENT')).toBe(colors.red[100]);
    expect(getServoStateColor('NO_SSH')).toBe(colors.red[100]);
    expect(getServoStateColor('NOT_CONNECTED')).toBe(colors.red[100]);
    expect(getServoStateColor('SERVOD_ISSUE')).toBe(colors.red[100]);
    expect(getServoStateColor('CCD_TESTLAB_ISSUE')).toBe(colors.red[100]);
    expect(getServoStateColor('CR50_CONSOLE_MISSING')).toBe(colors.red[100]);
    expect(getServoStateColor('SBU_LOW_VOLTAGE')).toBe(colors.red[100]);
    expect(getServoStateColor('DUT_NOT_CONNECTED')).toBe(colors.red[100]);
    expect(getServoStateColor('NOT_APPLICABLE')).toBe(colors.transparent);
    expect(getServoStateColor('UNKNOWN')).toBe(colors.transparent);
    expect(getServoStateColor('')).toBe(colors.transparent);
  });

  test('should map Servo USB State values to semantic chip colors across healthy, degraded, neutral, and broken states', () => {
    expect(getServoUsbStateColor('NORMAL')).toBe(colors.green[100]);
    expect(getServoUsbStateColor('HARDWARE_NORMAL')).toBe(colors.green[100]);
    expect(getServoUsbStateColor('ACCEPTABLE')).toBe(colors.yellow[100]);
    expect(getServoUsbStateColor('HARDWARE_ACCEPTABLE')).toBe(
      colors.yellow[100],
    );
    expect(getServoUsbStateColor('NOT_DETECTED')).toBe(colors.red[100]);
    expect(getServoUsbStateColor('NEED_REPLACEMENT')).toBe(colors.red[100]);
    expect(getServoUsbStateColor('NOT_APPLICABLE')).toBe(colors.transparent);
    expect(getServoUsbStateColor('UNKNOWN')).toBe(colors.transparent);
    expect(getServoUsbStateColor('')).toBe(colors.transparent);
  });
});
