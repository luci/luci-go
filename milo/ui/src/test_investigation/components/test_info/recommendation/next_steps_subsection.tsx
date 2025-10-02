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

import { Box, CircularProgress, Link, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { StyledActionBlock } from '@/common/components/gm3_styled_components';
import { useTestVariantsClient } from '@/common/hooks/prpc_clients';
import { getStatusStyle } from '@/common/styles/status_styles';
import { Sources } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { QueryTestVariantStabilityRequest_TestVariantPosition } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { getSourcesFromInvocation } from '@/test_investigation/utils/test_info_utils';

import {
  useAssociatedBugs,
  useIsLoadingAssociatedBugs,
  useTestVariantBranch,
} from '../context';

import { getNextStepsInfo } from './next_steps_utills';

interface NextStepsSubsectionProps {
  expanded: boolean;
}

export function NextStepsSubsection({ expanded }: NextStepsSubsectionProps) {
  const associatedBugs = useAssociatedBugs();
  const isLoadingAssociatedBugs = useIsLoadingAssociatedBugs();
  const testVariant = useTestVariant();
  const project = useProject();
  const client = useTestVariantsClient();
  const invocation = useInvocation();
  const testVariantBranch = useTestVariantBranch();

  const sources = getSourcesFromInvocation(invocation);

  const testVariantReq =
    QueryTestVariantStabilityRequest_TestVariantPosition.fromPartial({
      testId: testVariant.testId,
      variant: {
        def: testVariant.variant?.def || {},
      },
      sources: Sources.fromPartial(sources || {}),
    });
  const { data } = useQuery({
    ...client.QueryStability.query({
      project: project || '',
      testVariants: [testVariantReq],
    }),
  });

  const nextStepsInfo = getNextStepsInfo(
    data,
    testVariant,
    invocation,
    testVariantBranch?.segments as Segment[],
  );
  return (
    <>
      {expanded ? (
        <Box
          sx={{
            flex: 1,
            padding: 2,
            borderRadius: '8px',
            background: 'var(--blue-50, #E8F0FE)',
          }}
        >
          {nextStepsInfo && (
            <Box
              sx={{
                mb: 1.5,
                padding: 1,
                borderRadius: '8px',
                background: '#FFF',
                gap: '10px',
                display: 'flex',
                flexDirection: 'column',
              }}
            >
              <Typography
                variant="body2"
                color={getStatusStyle(nextStepsInfo.status).textColor}
                sx={{
                  fontWeight: 'bold',
                }}
              >
                {nextStepsInfo.title}
              </Typography>
              <Typography variant="body2">{nextStepsInfo.subtitle}</Typography>
            </Box>
          )}
          {associatedBugs && associatedBugs.length > 0 ? (
            associatedBugs.map((bug) => (
              <StyledActionBlock
                severity="info"
                key={`track-${bug.system}-${bug.id}`}
                sx={{ mb: 1.5 }}
              >
                <Typography variant="body2">
                  <Link
                    href={bug.url || `https://${bug.system}.com/${bug.id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Track {bug.linkText || `${bug.system}/${bug.id}`}
                  </Link>
                </Typography>
              </StyledActionBlock>
            ))
          ) : isLoadingAssociatedBugs ? (
            <CircularProgress />
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{
                mb: 1.5,
                padding: 1,
                borderRadius: '8px',
                background: '#FFF',
              }}
            >
              No associated bugs found for next steps.
            </Typography>
          )}
        </Box>
      ) : (
        <>
          {nextStepsInfo && (
            <Box
              sx={{
                padding: 1,
                borderRadius: '8px',
                background: '#FFF',
                gap: '10px',
                display: 'flex',
                flexDirection: 'column',
              }}
            >
              <Typography
                variant="body2"
                color={getStatusStyle(nextStepsInfo.status).textColor}
                sx={{
                  fontWeight: 'bold',
                }}
              >
                {nextStepsInfo.title}
              </Typography>
            </Box>
          )}
        </>
      )}
    </>
  );
}
