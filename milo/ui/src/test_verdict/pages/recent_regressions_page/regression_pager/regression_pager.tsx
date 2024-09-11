// Copyright 2024 The LUCI Authors.
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

import { ArrowBackIos, ArrowForwardIos } from '@mui/icons-material';
import { Box, Button } from '@mui/material';
import { DateTime } from 'luxon';

import {
  DEFAULT_EXTRA_ZONE_CONFIGS,
  Timestamp,
} from '@/common/components/timestamp';
import { LONG_TIME_FORMAT } from '@/common/tools/time_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { getWeek, weekUpdater } from './regression_pager_utils';

const EXTRA_TIME_ZONES = {
  format: LONG_TIME_FORMAT,
  zones: [
    ...DEFAULT_EXTRA_ZONE_CONFIGS,
    {
      label: 'local',
      zone: 'system',
    },
  ],
};

export interface RegressionPagerProps {
  readonly now: DateTime;
}

export function RegressionPager({ now }: RegressionPagerProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const weekBegin = getWeek(searchParams, now);
  const weekEnd = weekBegin.plus({ weeks: 1 });
  return (
    <Box
      display="flex"
      alignItems="center"
      sx={{ fontSize: '20px', gap: '10px' }}
    >
      <Button
        onClick={() =>
          setSearchParams(weekUpdater(weekBegin.minus({ weeks: 1 })))
        }
      >
        <ArrowBackIos />
      </Button>
      <Timestamp
        datetime={weekBegin.toUTC()}
        format={'ccc, MMM dd yyyy'}
        extraTimezones={EXTRA_TIME_ZONES}
      />
      <span>-</span>
      {weekEnd > now ? (
        <span>now</span>
      ) : (
        <Timestamp
          datetime={weekEnd.toUTC()}
          format={'ccc, MMM dd yyyy'}
          extraTimezones={EXTRA_TIME_ZONES}
        />
      )}
      <Button
        disabled={weekEnd > now}
        onClick={() => setSearchParams(weekUpdater(weekEnd))}
      >
        <ArrowForwardIos />
      </Button>
    </Box>
  );
}
