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

import { Box, Typography, type SxProps, type Theme } from '@mui/material';

import { HealthChip } from './health_chip';

export interface BaselineChipProps {
  /**
   * Whether the active baseline is availability/quota (true or 'expected')
   * or health/enrolled (false or 'enrolled').
   */
  readonly isAvailability?: boolean;
  readonly baselineType?: 'expected' | 'enrolled';
  /** Optional custom testId for testing assertions. */
  readonly testId?: string;
  /** Custom style overrides. */
  readonly sx?: SxProps<Theme>;
}

export const BaselineChip = ({
  isAvailability: isAvailabilityProp,
  baselineType,
  testId = 'baseline-chip',
  sx,
}: BaselineChipProps) => {
  const isAvailability =
    isAvailabilityProp !== undefined
      ? isAvailabilityProp
      : baselineType === 'expected';

  const label = isAvailability ? 'Quota Baseline' : 'Enrolled Baseline';
  const description = isAvailability ? (
    <>
      Compares ready devices against the <b>target quota</b> (how many devices
      we need). Active whenever the view has a defined quota target.
    </>
  ) : (
    <>
      Compares ready devices against <b>enrolled devices</b> (physical machines
      present in the lab). Active when viewing any group without a defined quota
      target.
    </>
  );

  return (
    <HealthChip
      label={label}
      color={isAvailability ? 'primary' : 'default'}
      testId={testId}
      sx={sx}
      tooltip={
        <Box sx={{ p: 0.5 }}>
          <Typography
            variant="caption"
            display="block"
            sx={{ fontWeight: 'bold', mb: 0.5 }}
          >
            {label}
          </Typography>
          <Typography variant="caption" display="block">
            {description}
          </Typography>
          <Typography
            variant="caption"
            display="block"
            color="text.secondary"
            sx={{ fontStyle: 'italic', mt: 0.5 }}
          >
            Note: Calculated based on devices in <b>dut_state = READY</b>.
          </Typography>
        </Box>
      }
    />
  );
};
