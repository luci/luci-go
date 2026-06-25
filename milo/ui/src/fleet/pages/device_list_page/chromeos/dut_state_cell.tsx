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

import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import {
  renderChipCell,
  StateUnion,
} from '@/fleet/components/table/cell_with_chip';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { FC_CellProps } from '@/fleet/types/table';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { ChromeOSDevice } from './chromeos_types';
import { getStatusColor } from './dut_state';

export const DutStateCell = ({
  params,
}: {
  params: FC_CellProps<ChromeOSDevice>;
}) => {
  const { trackEvent } = useGoogleAnalytics();
  const stateValue = String(params.cell.getValue() ?? '');
  const device = params.row.original;

  if (stateValue === '') {
    return <></>;
  }

  const chip = renderChipCell<ChromeOSDevice>({
    getValueOrUrl: getSwarmingStateDocLinkForLabel,
    getColor: getStatusColor,
    overrideValue: stateValue.toUpperCase() as StateUnion,
    getTrackingEvent: (value) => ({
      eventName: 'state_doc_link_clicked',
      payload: { componentName: 'dut_state', activeTab: value },
    }),
  })(params);

  if (stateValue !== 'RESERVED') {
    return <>{chip}</>;
  }

  const comment =
    device.deviceSpec?.labels?.['ufs.dut_state_reason']?.values?.[0] || '';

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        minWidth: 0,
        width: '100%',
        '& .MuiChip-root': {
          minWidth: 0,
          flex: '0 1 auto',
          '& .MuiChip-label': {
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          },
        },
      }}
    >
      {chip}
      <InfoTooltip
        fontSize="1.125rem"
        color="action.active"
        paperCss={{ maxWidth: 300 }}
        aria-label="show reservation details"
        onMouseEnter={() => {
          trackEvent('reserve_info_hovered', {
            componentName: 'reserve_info_button',
          });
        }}
        onFocus={() => {
          trackEvent('reserve_info_hovered', {
            componentName: 'reserve_info_button',
          });
        }}
      >
        <Box sx={{ p: 0.5 }}>
          <Typography
            variant="caption"
            sx={{
              display: 'block',
              opacity: 0.8,
              fontSize: '0.75rem',
              lineHeight: 1.2,
            }}
          >
            DUT is currently reserved for:
          </Typography>
          <Typography
            variant="body2"
            sx={{
              mt: 0.5,
              fontWeight: 500,
              wordBreak: 'break-word',
              lineHeight: 1.3,
            }}
          >
            {comment || 'No comment provided'}
          </Typography>
        </Box>
      </InfoTooltip>
    </Box>
  );
};
