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
import { createElement, JSX } from 'react';

import { getStatusStyle } from '@/common/styles/status_styles';

import { SegmentAnalysisResult } from '../../utils/test_info_utils';

import { RateBox } from './rate_box';

interface AnalysisSubsectionsProps {
  segmentAnalysis: SegmentAnalysisResult;
  allTestHistoryLink?: string;
}

export function AnalysisSubsection({
  segmentAnalysis,
  allTestHistoryLink,
}: AnalysisSubsectionsProps): JSX.Element {
  const currentRateBoxStyle = segmentAnalysis.currentRateBox
    ? getStatusStyle(segmentAnalysis.currentRateBox.statusType)
    : getStatusStyle('unknown'); // Fallback style

  const TransitionIconComponent = currentRateBoxStyle.icon;

  return (
    <Box sx={{ flex: 1 }}>
      <Typography
        variant="subtitle1"
        component="div"
        gutterBottom
        sx={{ fontWeight: 'medium' }}
      >
        Analysis
      </Typography>

      {/* Transition Text / Current Rate */}
      {segmentAnalysis.transitionText && segmentAnalysis.currentRateBox ? (
        <Typography
          variant="body2"
          sx={{ display: 'flex', alignItems: 'flex-start', mb: 1 }}
        >
          {TransitionIconComponent && (
            <TransitionIconComponent
              sx={{
                fontSize: 18,
                mr: 0.5,
                mt: '2px',
                color:
                  currentRateBoxStyle.iconColor ||
                  currentRateBoxStyle.textColor,
              }}
            />
          )}
          <span>{segmentAnalysis.transitionText}</span>
          {allTestHistoryLink && (
            <Link
              href={allTestHistoryLink}
              sx={{ ml: 0.5, whiteSpace: 'nowrap', alignSelf: 'center' }}
              target="_blank"
              rel="noopener noreferrer"
            >
              View all test history
            </Link>
          )}
        </Typography>
      ) : (
        segmentAnalysis.currentRateBox && (
          <Typography
            variant="body2"
            sx={{ display: 'flex', alignItems: 'center', mb: 1 }}
          >
            {currentRateBoxStyle.icon &&
              createElement(currentRateBoxStyle.icon, {
                sx: {
                  fontSize: 18,
                  mr: 1,
                  color:
                    currentRateBoxStyle.iconColor ||
                    currentRateBoxStyle.textColor,
                },
              })}
            Current <RateBox info={segmentAnalysis.currentRateBox} /> failure
            rate{' '}
            {segmentAnalysis.stabilitySinceText
              ? `since ${segmentAnalysis.stabilitySinceText}`
              : 'for some time'}
            .
            {allTestHistoryLink && (
              <Link
                href={allTestHistoryLink}
                sx={{ ml: 0.5, whiteSpace: 'nowrap' }}
                target="_blank"
                rel="noopener noreferrer"
              >
                View all test history
              </Link>
            )}
          </Typography>
        )
      )}
    </Box>
  );
}
