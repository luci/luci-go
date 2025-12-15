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
import { Box, Typography } from '@mui/material';
import { memo } from 'react';

import { SegmentSummary } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';

interface AndroidHistoryChangepointProps {
  pointingToSegment: SegmentSummary;
  olderSegment?: SegmentSummary;
}

/** Renders a changepoint in the history rate display. Renders as an arrow. */
export const AndroidHistoryChangepoint = memo(
  function AndroidHistoryChangepoint({
    pointingToSegment,
    olderSegment,
  }: AndroidHistoryChangepointProps) {
    const startBuildId = pointingToSegment.start_result?.build_id;

    // Fluxgate segments don't have "startHour" easily accessible for duration display
    // without fetching build details. For now, we omit the duration text.

    const fromId = olderSegment?.start_result?.build_id;
    const toId = pointingToSegment.end_result?.build_id;
    const blamelistUrl =
      fromId && toId
        ? `https://android-build.corp.google.com/range_search/cls/from_id/${fromId}/to_id/${toId}/`
        : undefined;

    return (
      <Box
        sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}
      >
        <Box sx={{ height: 'calc(1em * 1.2)' }}>
          {startBuildId && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{
                lineHeight: 'normal',
                textAlign: 'center',
                whiteSpace: 'nowrap',
              }}
            >
              {startBuildId}
            </Typography>
          )}
        </Box>
        {blamelistUrl ? (
          <a
            href={blamelistUrl}
            target="_blank"
            rel="noopener noreferrer"
            style={{ display: 'flex' }}
            title="View Blamelist"
          >
            <ArrowBackIcon
              data-testid="ArrowBackIcon"
              sx={{
                color: 'text.disabled',
                my: 0.25,
                fontSize: '20px',
                cursor: 'pointer',
                '&:hover': { color: 'primary.main' },
              }}
            />
          </a>
        ) : (
          <ArrowBackIcon
            data-testid="ArrowBackIcon"
            sx={{ color: 'text.disabled', my: 0.25, fontSize: '20px' }}
          />
        )}
      </Box>
    );
  },
);
