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
import { StateUnion } from '@/fleet/components/table/cell_with_chip';
import { ChipComponent } from '@/fleet/components/table/chip_component';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { getStatusColor } from './dut_state';

export interface DutStateCellProps {
  state?: string;
  comment?: string;
}

export const DutStateCell = ({
  state = '',
  comment = '',
}: DutStateCellProps) => {
  const { trackEvent } = useGoogleAnalytics();
  const stateValue = state.trim();

  if (stateValue === '') {
    return null;
  }

  const upperState = stateValue.toUpperCase() as StateUnion;

  const chip = (
    <ChipComponent
      label={upperState}
      url={getSwarmingStateDocLinkForLabel(upperState)}
      color={getStatusColor(upperState)}
      onClick={() => {
        trackEvent('state_doc_link_clicked', {
          componentName: 'dut_state',
          activeTab: upperState,
        });
      }}
    />
  );

  if (upperState !== 'RESERVED') {
    return <>{chip}</>;
  }

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
