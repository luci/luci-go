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

import { Box, ButtonBase, Link, Typography } from '@mui/material';
import { useCallback, useState } from 'react';

import { CLsPopover } from '../cls_popver';
import { useFormattedCLs } from '../context';
import { useSegmentAnalysis } from '../context/context';
import { RateBox } from '../rate_box';
import { NO_HISTORY_DATA_TEXT, NO_CLS_TEXT } from '../types';

const clHistoryLinkSuffix = '/+log';
const allTestHistoryLink = '#all-history-todo';

export function OverviewHistorySection() {
  const allFormattedCLs = useFormattedCLs();
  const segmentAnalysis = useSegmentAnalysis();
  const [clPopoverAnchorEl, setClPopoverAnchorEl] =
    useState<HTMLElement | null>(null);

  const handleCLPopoverOpen = useCallback(
    (event: React.MouseEvent<HTMLElement>) => {
      setClPopoverAnchorEl(event.currentTarget);
    },
    [],
  );

  const handleCLPopoverClose = () => {
    setClPopoverAnchorEl(null);
  };

  const clPopoverOpen = Boolean(clPopoverAnchorEl);

  return (
    <>
      <Typography variant="subtitle2" gutterBottom color="text.primary">
        History (failure rate):
      </Typography>
      {segmentAnalysis && segmentAnalysis.currentRateBox ? (
        <Box sx={{ mb: 1 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              mb: 0.5,
            }}
          >
            <RateBox info={segmentAnalysis.currentRateBox} />
            {segmentAnalysis.previousRateBox && (
              <>
                <Typography
                  sx={{
                    mx: 0.5,
                    color: 'text.disabled',
                    display: 'flex',
                    alignItems: 'center',
                  }}
                  aria-label="to"
                >
                  {'➔'}
                </Typography>
                <RateBox info={segmentAnalysis.previousRateBox} />
              </>
            )}
            <Link
              href={allTestHistoryLink}
              variant="caption"
              sx={{
                ml: 'auto',
                whiteSpace: 'nowrap',
                alignSelf: 'center',
              }}
            >
              View full history
            </Link>
          </Box>
        </Box>
      ) : (
        <Typography variant="body2" color="text.disabled" sx={{ mb: 1 }}>
          {NO_HISTORY_DATA_TEXT}
        </Typography>
      )}

      <Typography variant="body2" color="text.secondary" component="div">
        CL:{' '}
        {allFormattedCLs.length === 0 && (
          <Typography component="span" color="text.disabled">
            {NO_CLS_TEXT}
          </Typography>
        )}
        {allFormattedCLs.length === 1 && (
          <Link
            href={allFormattedCLs[0].url}
            target="_blank"
            rel="noopener noreferrer"
            underline="hover"
          >
            {allFormattedCLs[0].display}
          </Link>
        )}
        {allFormattedCLs.length > 1 && (
          <>
            <Link
              href={allFormattedCLs[0].url}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {allFormattedCLs[0].display}
            </Link>
            <ButtonBase
              onClick={handleCLPopoverOpen}
              aria-describedby="cl-popover-overview"
              sx={{
                ml: 0.5,
                textDecoration: 'underline',
                color: 'primary.main',
                cursor: 'pointer',
                typography: 'body2',
              }}
            >
              + {allFormattedCLs.length - 1} more
            </ButtonBase>
          </>
        )}
        {allFormattedCLs.length > 0 && allFormattedCLs[0].url && (
          <Link
            href={allFormattedCLs[0].url + clHistoryLinkSuffix}
            target="_blank"
            rel="noopener noreferrer"
            underline="hover"
            sx={{ ml: 0.5 }}
          >
            (View history)
          </Link>
        )}
        <CLsPopover
          anchorEl={clPopoverAnchorEl}
          open={clPopoverOpen}
          onClose={handleCLPopoverClose}
          id="cl-popover-overview"
        />
      </Typography>
    </>
  );
}
