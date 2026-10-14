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

import BluetoothIcon from '@mui/icons-material/Bluetooth';
import CableIcon from '@mui/icons-material/Cable';
import HeadphonesIcon from '@mui/icons-material/Headphones';
import WifiIcon from '@mui/icons-material/Wifi';
import Box from '@mui/material/Box';
import SvgIcon, { SvgIconProps } from '@mui/material/SvgIcon';
import Tooltip from '@mui/material/Tooltip';

import { colors } from '@/fleet/theme/colors';
import { PeripheralState } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';

/* eslint-disable max-len */
/**
 * ChameleonIcon represents the Chameleon hardware emulator in ChromeOS testbeds.
 * Uses a detailed chameleon silhouette with curly tail and eye cutout.
 */
export const ChameleonIcon = (props: SvgIconProps) => (
  <SvgIcon {...props} viewBox="0 0 512 512" data-testid="ChameleonIcon">
    <path
      d="M230.4 92.35C229.59 102.08 222.38 119.92 218.12 129.02C216.66 132.13 214.03 139.02 210.16 139.9C206.38 140.75 202.66 133.67 201.21 131.03C196.61 122.67 193.2 113.91 189.75 105.05C188.53 101.9 185.56 96.75 186.33 93.46C194.84 91.25 222.19 90.14 230.4 92.35ZM511.92 209.99C511.92 214.03 511.92 218.07 511.92 222.12C510.33 224.08 508.68 232.9 506.84 236.33C502.46 244.47 494.81 251.22 487.54 256.78C454.41 282.09 404.49 294.88 363.1 290.2C342.47 287.86 319.17 280.54 309.28 260.5C293.86 229.27 309.4 193.01 313.66 161.08C315.19 149.59 314.39 136.98 312.85 125.55C311.84 117.99 309.13 111.15 313.15 103.93C317.18 96.67 323.91 95.04 331.41 93.71C345.89 91.14 360.9 92.15 375.26 94.79C397.5 98.87 418.98 106.98 436.69 121.37C452.88 134.53 464.74 152.02 478.32 167.63C486.29 176.8 495.71 184.53 503.9 193.47C506.46 196.27 508.5 199.96 509.82 203.49C510.53 205.37 510.56 208.67 511.92 209.99ZM0.08 316.55C0.08 307.21 0.08 297.86 0.08 288.51C2.08 285.04 1.4 278.68 2.17 274.62C4.15 264.18 6.06 253.46 9.24 243.31C19.53 210.42 34.09 180.52 58.72 155.76C64.22 150.22 70.01 144.42 76.45 139.98C80.59 142.17 84.02 146.66 87.61 149.68C96.99 157.54 107.81 167.12 119.95 170.08C128.62 172.19 136.51 170.13 140.01 161.26C140.91 158.98 141.74 156.51 142.02 154.07C142.63 148.87 143.04 143.9 142.97 138.64C142.88 131.2 141.98 123.53 140.8 116.19C140.24 112.69 138.6 108.49 139.14 104.97C141.4 103.2 145.13 102.61 147.89 101.78C155.17 99.61 162.83 96.89 170.43 96.13C172.83 99.94 173.61 104.95 175.39 109.11C176.8 112.42 177.96 115.85 179.44 119.12C186.24 134.1 202.74 169.82 222.69 147.66C225.45 144.6 227.77 141.01 229.78 137.43C235 128.13 238.54 117.9 241.84 107.8C243.12 103.89 243.55 97.21 246.37 94.22C258.6 94.73 285.39 103.91 295.83 110.58C296.33 112.72 296.04 114.94 296.42 117.13C299 132.17 300.99 147.65 298.75 162.95C296.31 179.65 291.02 195.8 289.17 212.62C288.06 222.73 287.59 233.5 289.21 243.58C290.04 248.75 291.72 253.72 292.69 258.81C288.52 259.56 280.89 255.31 279.99 250.86C279.27 247.28 281.46 243.81 279.81 240.35C277.54 235.6 270.21 235.08 267.26 239.43C259.39 251.02 269.95 266.19 280.73 270.82C285.34 272.8 292.07 273.06 295.55 277.01C302.47 284.9 294.45 300.68 286.28 304.74C278.11 308.8 269.37 306.42 261.34 303.27C242.96 296.05 224.02 278.36 221.06 257.87C220.17 251.68 220.13 245.83 221.15 239.63C221.68 236.44 223.47 233.31 223.27 230.03C223.06 226.68 220.84 224.02 217.53 223.23C210.36 221.52 207.97 230.15 207.11 235.43C205.39 245.91 205.05 256.84 208.25 267.1C209.15 269.99 210.54 272.72 211.37 275.64C206.31 275.64 201.25 275.64 196.2 275.64C193.25 258.82 190.44 241.22 179.71 227.3C162.35 204.78 127.62 203.02 109.26 225.45C105.12 230.51 98.46 241.27 109.05 243.79C114.96 245.2 117.35 238.64 120.31 235.09C125.02 229.42 132.1 224.78 139.57 224.07C167.57 221.42 175.77 247.23 180.3 269.23C182.06 277.79 184.36 289.22 180.82 297.51C174.69 311.82 154.11 313.8 145.25 301.07C140.48 294.22 140 276.73 138.11 267.61C137.24 263.43 136.1 256.32 131.79 254.2C127.33 252.01 121.48 255.12 121.19 260.21C120.89 265.6 124.95 271 124.91 276.57C121.35 277.69 117.12 277.57 113.39 278.17C106.23 279.32 98.98 281.25 92.44 284.36C84.1 288.33 76.97 295.64 72.33 303.55C71 305.81 70.17 308.5 68.74 310.66C66.46 314.11 60.3 317.72 57.28 321.7C46.66 335.68 42.27 355.47 53.28 370.57C62.75 383.57 79.96 387.76 94.89 383.18C100.16 381.56 112.28 377.53 110.52 369.97C109.53 365.71 105.21 362.59 100.79 364.24C96.81 365.72 93.61 368.46 89.42 369.68C80.31 372.34 69.61 368.91 64.37 360.9C52.05 342.07 74.86 325.17 90.96 321.24C106.67 317.41 124.74 320.17 137.42 330.39C158.82 347.63 157.25 383.42 139.77 402.75C129.04 414.62 113.45 419.17 97.96 420.22C62.32 422.63 30.01 404.9 14.3 372.64C9.03 361.82 5.47 350.11 3.18 338.29C2.17 333.02 1.87 320.46 0.08 316.55ZM124.61 110.78C127.77 120.52 128.92 134.46 128.91 144.72C128.91 147.93 129.19 154.27 126.25 156.42C123.58 158.38 117.88 154.53 115.63 153.02C108.99 148.61 93.02 137.09 89.01 130.69C90.8 128.08 94.69 126.36 97.35 124.69C104.09 120.44 116.99 111.93 124.61 110.78ZM408.48 146.55C401.63 147.03 395.46 148.73 389.56 152.27C356.29 172.19 372.26 224.71 410.77 223.86C416.06 223.74 421.43 222.44 426.18 220.17C464 202.1 449.88 143.62 408.48 146.55ZM408.93 161.07C439.38 160.33 442.41 205.94 413.57 208.94C409.61 209.35 405.48 209.17 401.73 207.77C377.63 198.78 383.01 161.7 408.93 161.07ZM408.05 173.63C393.2 176.46 397.27 199.77 412.14 196.73C426.78 193.73 422.96 170.78 408.05 173.63ZM504.47 276.46C504.47 298.72 504.47 320.99 504.47 343.25C452 343.25 399.53 343.25 347.06 343.25C347.39 346.25 350.95 349.23 352.81 351.59C357.74 357.82 369.98 371.04 372.79 377.47C374.42 381.22 374.42 385.58 372.71 389.3C371.26 392.45 368.52 394.81 366.12 397.22C359.73 403.65 353.03 411.49 342.99 406.78C335.77 403.39 328.04 395.29 321.58 390.32C308.13 379.96 294.78 369.43 281.48 358.87C278.07 356.15 264.48 344.23 261.49 343.37C257 342.08 245.76 343.25 240.57 343.25C223.11 343.25 205.66 343.25 188.2 343.25C181.89 343.25 169.45 344.56 163.89 342.95C161.37 339.69 160.09 335.07 157.8 331.51C156.08 328.84 152.24 325.97 151.26 323.13C156.76 322.72 162.2 324.83 167.97 323.83C179.73 321.79 190.45 313.36 194.81 302.17C196.23 298.53 196.32 294.7 197.41 291.07C202.51 289.71 209.1 290.75 214.39 290.75C216.24 290.75 219.09 290.21 220.79 291C222.39 291.74 223.88 294.72 225.09 296.04C228.3 299.55 231.71 302.86 235.5 305.74C253.73 319.57 283.54 330.09 301.89 311.01C305.29 307.48 308.31 303.15 309.86 298.47C310.67 296.04 310.93 293.37 312.05 291.07C318.58 289.06 325.87 295.53 331.83 297.75C343.83 302.22 355.85 304.76 368.69 305.32C397.41 306.57 427.97 302.39 454.7 291.76C462.15 288.79 469.39 285.48 476.47 281.71C479.49 280.1 482.62 277.13 485.89 276.15C488.94 275.24 501.49 275.46 504.47 276.46ZM126.72 291.54C127.84 295.79 128.28 299.94 130.19 303.99C131.16 306.04 133.83 308.61 133.96 310.79C131.58 310.46 129.18 309.05 126.85 308.27C121.68 306.56 116.01 305.57 110.58 305.17C106.45 304.86 102.08 304.85 97.96 305.35C94.9 305.72 91.29 306.87 88.29 306.5C94.83 297.39 104.91 294.86 115.25 292.69C118.7 291.97 123.27 290.97 126.72 291.54Z"
      fill="currentColor"
      fillRule="evenodd"
    />
  </SvgIcon>
);
/* eslint-enable max-len */

export interface SinglePeripheralIconProps {
  label: string;
  state?: PeripheralState;
  icon: React.ReactElement;
  testId?: string;
}

/**
 * SinglePeripheralIcon renders an individual peripheral's semantic icon colored by health:
 * - Green for OK
 * - Red for BROKEN
 * - Orange for MISSING
 *
 * If the peripheral is NOT present / NOT_APPLICABLE / UNSPECIFIED on the device,
 * this component renders nothing (null) to keep the table compact and free of visual noise.
 */
export const SinglePeripheralIcon = ({
  label,
  state,
  icon,
  testId,
}: SinglePeripheralIconProps) => {
  if (
    state === undefined ||
    state === PeripheralState.PERIPHERAL_STATE_UNSPECIFIED ||
    state === PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE
  ) {
    return null;
  }

  let color: string;
  let statusText: string;

  switch (state) {
    case PeripheralState.PERIPHERAL_STATE_OK:
      color = colors.green[500];
      statusText = 'OK';
      break;
    case PeripheralState.PERIPHERAL_STATE_BROKEN:
      color = colors.red[600];
      statusText = 'BROKEN';
      break;
    case PeripheralState.PERIPHERAL_STATE_MISSING:
      color = colors.orange[600];
      statusText = 'MISSING';
      break;
    default:
      return null;
  }

  return (
    <Tooltip title={`${label}: ${statusText}`}>
      <Box
        component="span"
        data-testid={testId}
        sx={{
          display: 'inline-flex',
          alignItems: 'center',
          color,
          lineHeight: 0,
          '& svg': {
            fontSize: 18,
          },
        }}
      >
        {icon}
      </Box>
    </Tooltip>
  );
};

export interface PeripheralsCellProps {
  wifiState?: PeripheralState;
  bluetoothState?: PeripheralState;
  servoState?: PeripheralState;
  chameleonState?: PeripheralState;
  audioboxJackpluggerState?: PeripheralState;
}

/**
 * PeripheralsCell displays icons for all configured peripherals on a DUT.
 * Peripherals not present or not applicable to the device are omitted.
 * If no peripherals are configured at all, renders nothing (null).
 */
export const PeripheralsCell = ({
  wifiState,
  bluetoothState,
  servoState,
  chameleonState,
  audioboxJackpluggerState,
}: PeripheralsCellProps) => {
  const isAnyConfigured = [
    wifiState,
    bluetoothState,
    servoState,
    chameleonState,
    audioboxJackpluggerState,
  ].some(
    (state) =>
      state !== undefined &&
      state !== PeripheralState.PERIPHERAL_STATE_UNSPECIFIED &&
      state !== PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
  );

  if (!isAnyConfigured) {
    return null;
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
      <SinglePeripheralIcon
        label="Wi-Fi"
        state={wifiState}
        icon={<WifiIcon />}
        testId="wifi-indicator"
      />
      <SinglePeripheralIcon
        label="Bluetooth"
        state={bluetoothState}
        icon={<BluetoothIcon />}
        testId="bluetooth-indicator"
      />
      <SinglePeripheralIcon
        label="Servo"
        state={servoState}
        icon={<CableIcon />}
        testId="servo-indicator"
      />
      <SinglePeripheralIcon
        label="Chameleon"
        state={chameleonState}
        icon={<ChameleonIcon />}
        testId="chameleon-indicator"
      />
      <SinglePeripheralIcon
        label="Audiobox Jackplugger"
        state={audioboxJackpluggerState}
        icon={<HeadphonesIcon />}
        testId="audiobox-indicator"
      />
    </Box>
  );
};
