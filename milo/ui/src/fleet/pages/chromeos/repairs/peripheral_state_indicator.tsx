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

import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import RemoveIcon from '@mui/icons-material/Remove';
import WarningIcon from '@mui/icons-material/Warning';
import Box from '@mui/material/Box';
import Tooltip from '@mui/material/Tooltip';

import { colors } from '@/fleet/theme/colors';
import { PeripheralState } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';

interface IndicatorProps {
  label: string; // "Wi-Fi", "Bluetooth", "Servo"
  state?: PeripheralState;
}

export const SinglePeripheralIcon = ({ label, state }: IndicatorProps) => {
  switch (state) {
    case PeripheralState.PERIPHERAL_STATE_OK:
      return (
        <Tooltip title={`${label}: OK`}>
          <CheckCircleIcon sx={{ color: colors.green[500], fontSize: 18 }} />
        </Tooltip>
      );
    case PeripheralState.PERIPHERAL_STATE_BROKEN:
      return (
        <Tooltip title={`${label}: BROKEN`}>
          <WarningIcon sx={{ color: colors.red[600], fontSize: 18 }} />
        </Tooltip>
      );
    case PeripheralState.PERIPHERAL_STATE_MISSING:
      return (
        <Tooltip title={`${label}: MISSING`}>
          <WarningIcon sx={{ color: colors.orange[600], fontSize: 18 }} />
        </Tooltip>
      );
    case PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE:
      return (
        <Tooltip title={`${label}: N/A`}>
          <RemoveIcon sx={{ color: colors.grey[500], fontSize: 18 }} />
        </Tooltip>
      );
    case PeripheralState.PERIPHERAL_STATE_UNSPECIFIED:
    case undefined:
    default:
      return (
        <Tooltip title={`${label}: UNKNOWN`}>
          <HelpOutlineIcon sx={{ color: colors.grey[500], fontSize: 18 }} />
        </Tooltip>
      );
  }
};

export const PeripheralsCell = ({
  wifiState,
  bluetoothState,
  servoState,
}: {
  wifiState?: PeripheralState;
  bluetoothState?: PeripheralState;
  servoState?: PeripheralState;
}) => (
  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
    <SinglePeripheralIcon label="Wi-Fi" state={wifiState} />
    <SinglePeripheralIcon label="Bluetooth" state={bluetoothState} />
    <SinglePeripheralIcon label="Servo" state={servoState} />
  </Box>
);
