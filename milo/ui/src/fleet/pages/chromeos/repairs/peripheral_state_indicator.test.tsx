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

import WifiIcon from '@mui/icons-material/Wifi';
import { render, screen } from '@testing-library/react';

import { colors } from '@/fleet/theme/colors';
import { PeripheralState } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';

import {
  ChameleonIcon,
  PeripheralsCell,
  SinglePeripheralIcon,
} from './peripheral_state_indicator';

describe('ChameleonIcon', () => {
  it('renders lizard icon SVG element', () => {
    render(<ChameleonIcon />);
    expect(screen.getByTestId('ChameleonIcon')).toBeInTheDocument();
  });
});

describe('SinglePeripheralIcon', () => {
  it('renders green icon for OK state with correct tooltip', async () => {
    render(
      <SinglePeripheralIcon
        label="Wi-Fi"
        state={PeripheralState.PERIPHERAL_STATE_OK}
        icon={<WifiIcon />}
        testId="wifi-indicator"
      />,
    );
    const indicator = screen.getByTestId('wifi-indicator');
    expect(indicator).toBeInTheDocument();
    expect(indicator).toHaveStyle({ color: colors.green[500] });
    expect(screen.getByLabelText('Wi-Fi: OK')).toBeInTheDocument();
  });

  it('renders red icon for BROKEN state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Bluetooth"
        state={PeripheralState.PERIPHERAL_STATE_BROKEN}
        icon={<WifiIcon />}
        testId="bt-indicator"
      />,
    );
    const indicator = screen.getByTestId('bt-indicator');
    expect(indicator).toBeInTheDocument();
    expect(indicator).toHaveStyle({ color: colors.red[600] });
    expect(screen.getByLabelText('Bluetooth: BROKEN')).toBeInTheDocument();
  });

  it('renders orange icon for MISSING state with correct tooltip', () => {
    render(
      <SinglePeripheralIcon
        label="Servo"
        state={PeripheralState.PERIPHERAL_STATE_MISSING}
        icon={<WifiIcon />}
        testId="servo-indicator"
      />,
    );
    const indicator = screen.getByTestId('servo-indicator');
    expect(indicator).toBeInTheDocument();
    expect(indicator).toHaveStyle({ color: colors.orange[600] });
    expect(screen.getByLabelText('Servo: MISSING')).toBeInTheDocument();
  });

  it('does NOT render when state is NOT_APPLICABLE', () => {
    const { container } = render(
      <SinglePeripheralIcon
        label="Chameleon"
        state={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
        icon={<ChameleonIcon />}
        testId="chameleon-indicator"
      />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId('chameleon-indicator')).not.toBeInTheDocument();
  });

  it('does NOT render when state is UNSPECIFIED', () => {
    const { container } = render(
      <SinglePeripheralIcon
        label="Chameleon"
        state={PeripheralState.PERIPHERAL_STATE_UNSPECIFIED}
        icon={<ChameleonIcon />}
        testId="chameleon-indicator"
      />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId('chameleon-indicator')).not.toBeInTheDocument();
  });

  it('does NOT render when state is undefined', () => {
    const { container } = render(
      <SinglePeripheralIcon
        label="Servo"
        state={undefined}
        icon={<WifiIcon />}
        testId="servo-indicator"
      />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId('servo-indicator')).not.toBeInTheDocument();
  });
});

describe('PeripheralsCell', () => {
  it('renders semantic indicators for only configured peripherals', () => {
    render(
      <PeripheralsCell
        wifiState={PeripheralState.PERIPHERAL_STATE_OK}
        bluetoothState={PeripheralState.PERIPHERAL_STATE_BROKEN}
        servoState={PeripheralState.PERIPHERAL_STATE_OK}
        chameleonState={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
        audioboxJackpluggerState={undefined}
      />,
    );

    // Active peripherals are rendered
    expect(screen.getByTestId('wifi-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('bluetooth-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('servo-indicator')).toBeInTheDocument();

    expect(screen.getByLabelText('Wi-Fi: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: BROKEN')).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: OK')).toBeInTheDocument();

    // Non-configured peripherals are NOT rendered
    expect(screen.queryByTestId('chameleon-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('audiobox-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('no-peripherals')).not.toBeInTheDocument();
  });

  it('renders all 5 peripherals when all are configured on the DUT', () => {
    render(
      <PeripheralsCell
        wifiState={PeripheralState.PERIPHERAL_STATE_OK}
        bluetoothState={PeripheralState.PERIPHERAL_STATE_OK}
        servoState={PeripheralState.PERIPHERAL_STATE_MISSING}
        chameleonState={PeripheralState.PERIPHERAL_STATE_OK}
        audioboxJackpluggerState={PeripheralState.PERIPHERAL_STATE_BROKEN}
      />,
    );

    expect(screen.getByTestId('wifi-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('bluetooth-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('servo-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('chameleon-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('audiobox-indicator')).toBeInTheDocument();

    expect(screen.getByLabelText('Wi-Fi: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Servo: MISSING')).toBeInTheDocument();
    expect(screen.getByLabelText('Chameleon: OK')).toBeInTheDocument();
    expect(
      screen.getByLabelText('Audiobox Jackplugger: BROKEN'),
    ).toBeInTheDocument();
  });

  it('renders nothing when no peripherals are configured on the device', () => {
    const { container } = render(
      <PeripheralsCell
        wifiState={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
        bluetoothState={undefined}
        servoState={PeripheralState.PERIPHERAL_STATE_UNSPECIFIED}
        chameleonState={PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE}
        audioboxJackpluggerState={undefined}
      />,
    );

    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId('wifi-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('bluetooth-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('servo-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('chameleon-indicator')).not.toBeInTheDocument();
    expect(screen.queryByTestId('audiobox-indicator')).not.toBeInTheDocument();
  });
});
