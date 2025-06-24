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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { Box, Link, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { memo } from 'react';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { formatSegmentTimestamp } from './util';

interface HistoryChangepointProps {
  pointingToSegment: Segment;
  nowDtForFormatting: DateTime;
  blamelistBaseUrl: string | undefined;
}

/** Renders a changepoint in the history rate display. Renders as an arrow with a time and a blamelist link. */
export const HistoryChangepoint = memo(function HistoryChangepoint({
  pointingToSegment,
  nowDtForFormatting,
  blamelistBaseUrl,
}: HistoryChangepointProps) {
  const blamelistLink = `${blamelistBaseUrl}#CP-${pointingToSegment.startPosition}`;

  const durationText =
    formatSegmentTimestamp(pointingToSegment.startHour, nowDtForFormatting) ||
    '';
  const durationDisplay = durationText.trim() ? (
    <Typography
      variant="caption"
      color="text.secondary"
      sx={{ lineHeight: 'normal', textAlign: 'center', whiteSpace: 'nowrap' }}
    >
      {durationText}
    </Typography>
  ) : (
    <Box sx={{ height: 'calc(1em * 1.2)' }} /> // Approx height of a caption line to prevent collapse
  );

  return (
    <Box
      sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}
    >
      {durationDisplay}
      <ArrowBackIcon
        data-testid="ArrowBackIcon"
        sx={{ color: 'text.disabled', my: 0.25, fontSize: '20px' }}
      />
      {blamelistBaseUrl && (
        <Link
          href={blamelistLink}
          target="_blank"
          rel="noopener noreferrer"
          variant="caption"
          sx={{ lineHeight: 'normal', whiteSpace: 'nowrap' }}
        >
          blamelist
        </Link>
      )}
    </Box>
  );
});
