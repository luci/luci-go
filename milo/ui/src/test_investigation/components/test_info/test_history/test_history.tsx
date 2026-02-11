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
import InfoOutlineIcon from '@mui/icons-material/InfoOutlined';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';
import { useState } from 'react';

import { useTestVariantBranch } from '../context';

import { TestHistorySegment } from './test_history_segment';
export function TestHistory() {
  const testVariantBranch = useTestVariantBranch();
  const [hasExpandedSegment, setHasExpandedSegment] = useState(false);

  const expandOneSegment = () => {
    setHasExpandedSegment(true);
  };

  const collapseAllSegments = () => {
    setHasExpandedSegment(false);
  };

  const testFailureRateInfo = `
  The failure rate is not based on a fixed time window or number of runs.
  Instead, it looks at the current failure rate and determines if new test runs deviate from the expected value.
  If it does, then it will create a new segment and move the current failure rate to the previous slot.
  `;

  return (
    <Box sx={{ minHeight: '400px', p: 2 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          gap: '4px',
        }}
      >
        {!hasExpandedSegment ? (
          <Typography variant="h6">Test history</Typography>
        ) : (
          <>
            <IconButton onClick={collapseAllSegments} sx={{ p: 0 }}>
              <ArrowBackIcon
                sx={{ color: 'grey.900', width: '24px', p: 0 }}
              ></ArrowBackIcon>
            </IconButton>
            <Typography variant="h6">Collapse all</Typography>
          </>
        )}
      </Box>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'flex-start',
          mt: 3,
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            mt: 1,
            mr: 2,
            minWidth: '125px',
          }}
        >
          <Typography
            variant="body2"
            sx={{ height: '72px', color: 'text.secondary', pt: 1 }}
          >
            Build
          </Typography>
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
              gap: '4px',
            }}
          >
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
              }}
            >
              Test failure rate
            </Typography>
            <Tooltip title={testFailureRateInfo}>
              <InfoOutlineIcon
                sx={{ width: '16px', color: 'text.secondary' }}
              ></InfoOutlineIcon>
            </Tooltip>
          </Box>
        </Box>
        <Box sx={{ display: 'flex', flexDirection: 'row', mt: 1 }}>
          {testVariantBranch?.segments.map((segment, index) => (
            <TestHistorySegment
              setExpandedTestHistory={expandOneSegment}
              testHistoryHasExpandedSegment={hasExpandedSegment}
              segment={segment}
              key={index}
              isStartSegment={index === 0 ? true : false}
              isEndSegment={
                index === testVariantBranch.segments.length - 1 ? true : false
              }
            ></TestHistorySegment>
          ))}
        </Box>
      </Box>
    </Box>
  );
}
