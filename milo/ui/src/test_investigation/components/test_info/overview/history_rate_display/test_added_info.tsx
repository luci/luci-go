// Copyright 2025 The LUCI Authors.
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

import { Box, Link, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { memo } from 'react';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { formatSegmentTimestamp } from './util';

interface TestAddedInfoProps {
  segment: Segment;
  nowDtForFormatting: DateTime;
  blamelistBaseUrl: string | undefined;
}

/**
 * A component that displays when a test was added and a link to the blamelist.
 * Having an explicit textual display makes this common condition (a test failing since it was added)
 * more likely to be understood by new users.
 */
export const TestAddedInfo = memo(function TestAddedDisplay({
  segment,
  nowDtForFormatting,
  blamelistBaseUrl,
}: TestAddedInfoProps) {
  const startTimeAgo = formatSegmentTimestamp(
    segment.startHour,
    nowDtForFormatting,
  );
  const blamelistLink = `${blamelistBaseUrl}#CP-${segment.startPosition}`;

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
        p: 1,
      }}
    >
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ lineHeight: 'normal', whiteSpace: 'nowrap' }}
      >
        Test added {startTimeAgo}
      </Typography>
      {blamelistBaseUrl && (
        <Link
          href={blamelistLink}
          target="_blank"
          rel="noopener noreferrer"
          variant="caption"
          sx={{ lineHeight: 'normal', whiteSpace: 'nowrap', mt: 0.25 }}
        >
          blamelist
        </Link>
      )}
    </Box>
  );
});
