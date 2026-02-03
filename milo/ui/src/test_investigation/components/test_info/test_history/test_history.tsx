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
import { Box, IconButton, Typography } from '@mui/material';
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

  return (
    <Box sx={{ minHeight: '400px', p: 1.5 }}>
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
      <Box sx={{ display: 'flex', flexDirection: 'row', mt: 1 }}>
        {testVariantBranch?.segments.map((segment, index) => {
          return (
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
          );
        })}
      </Box>
    </Box>
  );
}
