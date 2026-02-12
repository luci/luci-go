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

import { Box, CircularProgress, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { getStatusStyle } from '@/common/styles/status_styles';
import {
  QuerySourceVerdictsResponse_SourceVerdict,
  QuerySourceVerdictsResponse_VerdictStatus,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { BatchGetTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { useTestVariantBranch } from '../context';

const getSemanticStatus = (
  status: QuerySourceVerdictsResponse_VerdictStatus,
) => {
  switch (status) {
    case QuerySourceVerdictsResponse_VerdictStatus.EXPECTED:
      return 'passed';
    case QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED:
      return 'failed';
    case QuerySourceVerdictsResponse_VerdictStatus.FLAKY:
      return 'flaky';
    case QuerySourceVerdictsResponse_VerdictStatus.SKIPPED:
      return 'skipped';
    default:
      return 'unknown';
  }
};

interface SourceVerdictTooltipProps {
  sourceVerdict: QuerySourceVerdictsResponse_SourceVerdict;
  isFailing?: boolean;
}

/**
 * An informative tooltip giving the user more details about a segment in the history display.
 */
export function SourceVerdictTooltip({
  sourceVerdict,
}: SourceVerdictTooltipProps) {
  const testVerdict = sourceVerdict.verdicts[0];
  const invocationId = testVerdict.invocationId;
  const resultDbClient = useResultDbClient();
  const testVariantBranch = useTestVariantBranch();

  const { data: response, isLoading } = useQuery({
    ...resultDbClient.BatchGetTestVariants.query(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: `invocations/${invocationId}`,
        testVariants: [
          {
            testId: testVariantBranch?.testId,
            variantHash: testVariantBranch?.variantHash,
          },
        ],
      }),
    ),
  });

  const semanticStatus = getSemanticStatus(sourceVerdict.status);
  const style = getStatusStyle(semanticStatus);
  const IconComponent = style.icon;

  const errorMessage =
    response?.testVariants[0]?.results[0]?.result?.failureReason
      ?.primaryErrorMessage || '';

  if (isLoading) {
    return <CircularProgress></CircularProgress>;
  }

  return (
    <Box sx={{ p: 1, display: 'flex', flexDirection: 'column', gap: '12px' }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          p: 0,
          gap: '4px',
        }}
      >
        {IconComponent && (
          <IconComponent
            sx={{
              color: style.iconColor || style.textColor,
            }}
          />
        )}
        <Typography variant="h5">
          {semanticStatus.charAt(0).toUpperCase() + semanticStatus.slice(1)}
        </Typography>
      </Box>
      <Box sx={{ display: 'flex', flexDirection: 'column' }}>
        <Typography variant="caption">
          Build{' '}
          <Typography variant="caption" sx={{ color: 'primary.main' }}>
            {sourceVerdict.position}
          </Typography>
        </Typography>
        {sourceVerdict.verdicts[0].partitionTime && (
          <Typography
            variant="caption"
            sx={{ fontStyle: 'italic', color: 'text.secondary' }}
          >
            Built at{' '}
            {new Date(
              sourceVerdict?.verdicts[0]?.partitionTime,
            ).toLocaleString()}
          </Typography>
        )}
      </Box>
      {sourceVerdict.status ===
        QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED && (
        <Box>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            Failure Reason:
          </Typography>
          <Typography
            variant="caption"
            sx={{ fontStyle: 'italic', color: 'text.secondary' }}
          >
            {errorMessage}
          </Typography>
        </Box>
      )}
    </Box>
  );
}
