// Copyright 2023 The LUCI Authors.
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

import { Alert, Box } from '@mui/material';
import { DateTime } from 'luxon';

import {
  isCanary,
  isFailureStatus,
  isTerminalStatus,
} from '@/build/tools/build_utils';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { parseProtoDuration } from '@/common/tools/time_utils';

import { useBuild } from '../../hooks';

export function AlertsSection() {
  const build = useBuild();
  if (!build) {
    return <></>;
  }

  const scheduledToBeCanceled =
    build.gracePeriod && build.cancelTime && !isTerminalStatus(build.status);
  const scheduledCancelTime = scheduledToBeCanceled
    ? DateTime.fromISO(build.cancelTime)
        .plus(parseProtoDuration(build.gracePeriod))
        // Add min_update_interval (currently always 30s).
        // TODO(crbug/1299302): read min_update_interval from buildbucket.
        .plus({ seconds: 30 })
    : null;

  return (
    <Box sx={{ marginBottom: '10px' }}>
      {isCanary(build) && isFailureStatus(build.status) && (
        <Alert severity="warning">
          This build ran on a canary version of LUCI. If you suspect it failed
          due to infra, retry the build. Next time it may use the non-canary
          version.
        </Alert>
      )}
      {build.statusDetails?.resourceExhaustion &&
        isFailureStatus(build.status) && (
          <Alert severity="error">
            This build failed due to resource exhaustion.
          </Alert>
        )}
      {build.statusDetails?.timeout && isFailureStatus(build.status) && (
        <Alert severity="error">This build timed out.</Alert>
      )}
      {scheduledCancelTime && (
        <Alert severity="info">
          This build was scheduled to be canceled
          {build.canceledBy ? ' by ' + build.canceledBy : ''}{' '}
          <RelativeTimestamp timestamp={scheduledCancelTime} />.
        </Alert>
      )}
    </Box>
  );
}
