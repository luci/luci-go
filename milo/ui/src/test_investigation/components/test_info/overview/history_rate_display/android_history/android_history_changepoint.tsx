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
import { Box } from '@mui/material';
import { memo } from 'react';

import { SegmentSummary } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';

export interface AndroidHistoryChangepointProps {
  pointingToSegment: SegmentSummary;
  olderSegment: SegmentSummary;
}

/** Renders as an arrow. */
export const AndroidHistoryChangepoint = memo(
  function AndroidHistoryChangepoint({
    pointingToSegment,
    olderSegment,
  }: AndroidHistoryChangepointProps) {
    const fromId = olderSegment.startResult?.buildId;
    const toId = pointingToSegment.endResult?.buildId;

    const url =
      fromId && toId
        ? `https://android-build.corp.google.com/range_search/cls/from_id/${fromId}/to_id/${toId}/`
        : null;

    return (
      <Box
        sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}
      >
        {url ? (
          <a
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            title="View Blamelist"
            style={{ display: 'flex', alignItems: 'center' }}
          >
            <ArrowBackIcon
              data-testid="ArrowBackIcon"
              sx={{ color: 'action.active', my: 0.25, fontSize: '20px' }}
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
