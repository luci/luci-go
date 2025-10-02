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

import { Box, CircularProgress, Paper, Typography } from '@mui/material';
import { useMemo } from 'react';

import {
  getStatusStyle,
  semanticStatusForTestVariant,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

interface InvocationSummaryProps {
  invocation: AnyInvocation;
  testVariants: readonly TestVariant[] | undefined;
  isLoadingTestVariants: boolean;
}

/**
 * Renders a summary of test variant counts by status for an invocation.
 * It fetches test variant counts and displays them in themed boxes.
 */
export function InvocationCounts({
  testVariants,
  isLoadingTestVariants,
}: InvocationSummaryProps) {
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    if (!testVariants) {
      return counts;
    }
    for (const variant of testVariants) {
      const status = semanticStatusForTestVariant(variant);
      counts[status] = (counts[status] || 0) + 1;
    }
    return counts;
  }, [testVariants]);

  if (isLoadingTestVariants) {
    return (
      <CircularProgress size={24} sx={{ display: 'block', margin: 'auto' }} />
    );
  }

  return (
    <Box
      sx={{
        display: 'flex',
        gap: 1.5,
        flexWrap: 'wrap',
        alignItems: 'center',
      }}
    >
      {[
        'Failed',
        'Execution Errored',
        'Exonerated',
        'Flaky',
        'Precluded',
        'Passed',
        'Skipped',
      ].map((label) => {
        const status = label
          .toLowerCase()
          .replace(' ', '_') as SemanticStatusType;
        const count = statusCounts[status] || 0;
        if (count === 0) return null;
        const styles = getStatusStyle(status);

        return (
          <ResultCountBox
            key={status}
            style={styles}
            label={label}
            count={'' + count}
          />
        );
      })}
    </Box>
  );
}

interface ResultCountBoxProps {
  label: string;
  count: string;
  style: StatusStyle;
}

function ResultCountBox({ label, count, style }: ResultCountBoxProps) {
  const Icon = style.icon;
  return (
    <Paper
      variant="outlined"
      sx={{
        p: 1,
        backgroundColor: style.backgroundColor,
        borderColor: style.borderColor,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        {Icon && (
          <Icon
            sx={{
              fontSize: '18px',
              color: style.onBackgroundColor,
            }}
          />
        )}
        <Typography
          variant="body2"
          sx={{
            color: style.onBackgroundColor,
            whiteSpace: 'nowrap',
          }}
        >
          {label}: {count === undefined ? 'Loading...' : count}
        </Typography>
      </Box>
    </Paper>
  );
}
