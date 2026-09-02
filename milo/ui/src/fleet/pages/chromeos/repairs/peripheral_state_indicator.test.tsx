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

import { PeripheralState } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';

import {
  PeripheralsCell,
  SinglePeripheralIcon,
} from './peripheral_state_indicator';

describe('SinglePeripheralIcon', () => {
  it('renders green check circle for OK state with correct tooltip', async () => {
    render(
      <SinglePeripheralIcon
        label="Wi-Fi"
        state={PeripheralState.PERIPHERAL_STATE_OK}
      />,
    );
    const icon = screen.getByTestId('CheckCircleIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Wi-Fi: OK')).toBeInTheDocument();
  });

  it('renders red warning icon for BROKEN state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Bluetooth"
        state={PeripheralState.PERIPHERAL_STATE_BROKEN}
      />,
    );
    const icon = screen.getByTestId('WarningIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: BROKEN')).toBeInTheDocument();
  });

  it('renders orange warning icon for MISSING state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Servo"
        state={PeripheralState.PERIPHERAL_STATE_MISSING}
      />,
    );
    const icon = screen.getByTestId('WarningIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: MISSING')).toBeInTheDocument();
  });

  it('renders grey remove icon for NOT_APPLICABLE state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Wi-Fi"
        state={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
      />,
    );
    const icon = screen.getByTestId('RemoveIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Wi-Fi: N/A')).toBeInTheDocument();
  });

  it('renders grey help outline icon for UNSPECIFIED state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Bluetooth"
        state={PeripheralState.PERIPHERAL_STATE_UNSPECIFIED}
      />,
    );
    const icon = screen.getByTestId('HelpOutlineIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: UNKNOWN')).toBeInTheDocument();
  });

  it('renders grey help outline icon for undefined state with correct tooltip', () => {
    render(<SinglePeripheralIcon label="Servo" state={undefined} />);
    const icon = screen.getByTestId('HelpOutlineIcon');
    expect(icon).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: UNKNOWN')).toBeInTheDocument();
  });
});

describe('PeripheralsCell', () => {
  it('renders Wi-Fi, Bluetooth, and Servo indicators in correct W / B / S order', () => {
    render(
      <PeripheralsCell
        wifiState={PeripheralState.PERIPHERAL_STATE_OK}
        bluetoothState={PeripheralState.PERIPHERAL_STATE_BROKEN}
        servoState={PeripheralState.PERIPHERAL_STATE_MISSING}
      />,
    );
    expect(screen.getByTestId('CheckCircleIcon')).toBeInTheDocument();
    const warningIcons = screen.getAllByTestId('WarningIcon');
    expect(warningIcons).toHaveLength(2);

    expect(screen.getByLabelText('Wi-Fi: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: BROKEN')).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: MISSING')).toBeInTheDocument();
  });

  it('handles undefined or N/A peripheral states gracefully across the triad', () => {
    render(
      <PeripheralsCell
        wifiState={undefined}
        bluetoothState={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
        servoState={PeripheralState.PERIPHERAL_STATE_UNSPECIFIED}
      />,
    );
    expect(screen.getByTestId('RemoveIcon')).toBeInTheDocument();
    const helpIcons = screen.getAllByTestId('HelpOutlineIcon');
    expect(helpIcons).toHaveLength(2);

    expect(screen.getByLabelText('Wi-Fi: UNKNOWN')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: N/A')).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: UNKNOWN')).toBeInTheDocument();
  });
});
