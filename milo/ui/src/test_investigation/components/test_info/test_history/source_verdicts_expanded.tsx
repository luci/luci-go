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
import { CheckCircle } from '@mui/icons-material';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import ErrorIcon from '@mui/icons-material/Error';
import MoreHorizIcon from '@mui/icons-material/MoreHoriz';
import {
  Box,
  Button,
  ButtonGroup,
  Link,
  Tooltip,
  Typography,
} from '@mui/material';
import CircularProgress from '@mui/material/CircularProgress';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { useTestHistoryClient } from '@/common/hooks/prpc_clients';
import {
  QuerySourceVerdictsV2Request,
  SourceVerdict,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { getSourcesFromInvocation } from '@/test_investigation/utils/test_info_utils';

import { useTestVariantBranch } from '../context';

import { SourceVerdictTooltip } from './source_verdict_tooltip';

const CurrentResultTooltip = (pos: string) => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        width: '70px',
      }}
    >
      <Typography variant="caption" align="center">
        This result on {pos}
      </Typography>
      <ArrowDropDownIcon sx={{ width: '16px', height: '16px', p: 0 }} />
    </Box>
  );
};

interface SourceVerdictsExpandedProps {
  segment: Segment;
  sourceVerdictNumberChanged: (num: number) => void;
}

const getEndSourceVerdictFromPosition = (
  pos: string,
  endSourceVerdicts: SourceVerdict[],
) => {
  if (!endSourceVerdicts) return null;
  for (let i = 0; i < endSourceVerdicts.length; i++) {
    if (endSourceVerdicts[i].position === pos) {
      return endSourceVerdicts[i];
    }
  }
  return null;
};

const getStartSourceVerdictFromPosition = (
  pos: string,
  startSourceVerdicts: SourceVerdict[],
) => {
  if (!startSourceVerdicts) return null;
  for (let i = startSourceVerdicts.length - 1; i > 0; i--) {
    if (startSourceVerdicts[i].position === pos) {
      return startSourceVerdicts[i];
    }
  }
  return null;
};

const isFailingSourceVerdict = (
  pos: string,
  endSourceVerdicts: SourceVerdict[],
  startSourceVerdicts: SourceVerdict[],
) => {
  if (
    !endSourceVerdicts ||
    !startSourceVerdicts ||
    endSourceVerdicts.length <= 0 ||
    startSourceVerdicts.length <= 0
  ) {
    return false;
  }
  if (
    Number(pos) >=
    Number(endSourceVerdicts?.[endSourceVerdicts.length - 1].position)
  ) {
    return (
      getEndSourceVerdictFromPosition(pos, endSourceVerdicts)?.status ===
      TestVerdict_Status.FAILED
    );
  }
  return (
    getStartSourceVerdictFromPosition(pos, startSourceVerdicts)?.status ===
    TestVerdict_Status.FAILED
  );
};

const PassSourceVerdictBox = (blamelistLink: string) => {
  return (
    <Box
      sx={{
        backgroundColor: 'var(--gm3-color-success)',
        display: 'flex',
        borderRadius: 0,
        minWidth: '82px',
        minHeight: '72px',
        width: '82px',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 0,
      }}
    >
      <Typography
        variant="subtitle1"
        sx={{
          color: 'white',
          fontWeight: 'bold',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '4px',
        }}
        component={Link}
        href={blamelistLink}
        target="_blank"
        rel="noopener noreferrer"
      >
        <CheckCircle sx={{ width: '20px' }} /> P
      </Typography>
    </Box>
  );
};

const FailSourceVerdictBox = (blamelistLink: string) => {
  return (
    <Box
      sx={{
        backgroundColor: 'var(--gm3-color-error)',
        display: 'flex',
        borderRadius: 0,
        minWidth: '82px',
        height: '72px',
        width: '82px',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 0,
      }}
      component={Link}
      href={blamelistLink}
      target="_blank"
      rel="noopener noreferrer"
    >
      <Typography
        variant="subtitle1"
        sx={{
          color: 'white',
          fontWeight: 'bold',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '4px',
        }}
      >
        <ErrorIcon sx={{ width: '20px' }} /> F
      </Typography>
    </Box>
  );
};

const PassFailSourceVerdictBox = (
  sourceVerdict: SourceVerdict,
  endSourceVerdicts: SourceVerdict[],
  startSourceVerdicts: SourceVerdict[],
  currentTestResultPosition: string | undefined,
  blamelistLink: string,
) => {
  if (!endSourceVerdicts || !startSourceVerdicts) {
    return <></>;
  }
  return (
    <>
      {currentTestResultPosition &&
      sourceVerdict.position === currentTestResultPosition ? (
        <Tooltip
          title={CurrentResultTooltip(sourceVerdict.position)}
          open
          placement="top"
          slotProps={{
            popper: {
              modifiers: [
                {
                  name: 'offset',
                  options: {
                    offset: [0, -14],
                  },
                },
              ],
            },
            tooltip: {
              sx: {
                bgcolor: 'transparent',
                color: 'text.secondary',
              },
            },
          }}
        >
          <HtmlTooltip
            title={
              <SourceVerdictTooltip
                sourceVerdict={sourceVerdict}
              ></SourceVerdictTooltip>
            }
          >
            <Link
              sx={{
                width: '8px',
                height: '60px',
                backgroundColor: isFailingSourceVerdict(
                  sourceVerdict.position,
                  endSourceVerdicts,
                  startSourceVerdicts,
                )
                  ? 'var(--gm3-color-error)'
                  : 'var(--gm3-color-success)',
                display: 'flex',
              }}
              href={blamelistLink}
              target="_blank"
              rel="noopener noreferrer"
            ></Link>
          </HtmlTooltip>
        </Tooltip>
      ) : (
        <HtmlTooltip
          title={
            <SourceVerdictTooltip
              sourceVerdict={sourceVerdict}
            ></SourceVerdictTooltip>
          }
        >
          <Link
            sx={{
              width: '8px',
              height: '60px',
              backgroundColor: isFailingSourceVerdict(
                sourceVerdict.position,
                endSourceVerdicts,
                startSourceVerdicts,
              )
                ? 'var(--gm3-color-error)'
                : 'var(--gm3-color-success)',
              display: 'flex',
            }}
            href={blamelistLink}
            target="_blank"
            rel="noopener noreferrer"
          ></Link>
        </HtmlTooltip>
      )}
    </>
  );
};

export function SourceVerdictsExpanded({
  segment,
  sourceVerdictNumberChanged,
}: SourceVerdictsExpandedProps) {
  const testVariantBranch = useTestVariantBranch();
  const thClient = useTestHistoryClient();
  const invocation = useInvocation() as Invocation;
  const testVariant = useTestVariant();
  const project = useProject();
  const sources = getSourcesFromInvocation(invocation);
  // How many source verdicts shown (from both start and end position) when component is expanded.
  const [sourceVerdictCount, setSourceVerdictCount] = useState(15);
  const testId = testVariant.testId;
  const variantHash = testVariant.variantHash;
  const refHash = testVariantBranch?.refHash;
  const currentTestResultPosition = sources?.gitilesCommit?.position;

  const blamelistBaseUrl = refHash
    ? `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`
    : undefined;

  const createBlamelistLink = (position: string) => {
    return `${blamelistBaseUrl}?expand=${`CP-${position}`}#CP-${position}`;
  };

  // Get most recent 1000 source verdicts, starting from end source position.
  const { data: endVerdictsResponse, isLoading: isLoadingEndVerdicts } =
    useQuery({
      ...thClient.QuerySourceVerdicts.query(
        QuerySourceVerdictsV2Request.fromPartial({
          project: testVariantBranch?.project,
          testIdFlat: {
            testId: testVariantBranch?.testId,
            variantHash: testVariantBranch?.variantHash,
          },
          sourceRefHash: testVariantBranch?.refHash,
          filter:
            'position < ' +
            segment.endPosition +
            ' AND position > ' +
            segment.startPosition,
        }),
      ),
    });

  let endSourceVerdicts = endVerdictsResponse?.sourceVerdicts;

  // If there are more than 1000 segments, also load from start source position, as this means there are still more verdicts in the segment.
  // Don't include those already in end source verdicts.
  const { data: startVerdictsResponse, isLoading: isLoadingStartVerdicts } =
    useQuery({
      ...thClient.QuerySourceVerdicts.query(
        QuerySourceVerdictsV2Request.fromPartial({
          project: testVariantBranch?.project,
          testIdFlat: {
            testId: testVariantBranch?.testId,
            variantHash: testVariantBranch?.variantHash,
          },
          sourceRefHash: testVariantBranch?.refHash,
          filter:
            'position < ' +
            endSourceVerdicts?.[endSourceVerdicts?.length - 1].position +
            ' AND position > ' +
            segment.startPosition,
        }),
      ),
      enabled:
        !isLoadingEndVerdicts &&
        !!endVerdictsResponse &&
        endSourceVerdicts &&
        endSourceVerdicts?.length >= 1000,
    });

  // Wait until all start and end verdicts have been fetched, otherwise the pass/fail will potentially be wrong.
  if (isLoadingEndVerdicts || isLoadingStartVerdicts) {
    return (
      <Box
        sx={{
          height: '72px',
          width: '206px',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
        }}
      >
        <CircularProgress></CircularProgress>
      </Box>
    );
  }

  // Combine end and start source verdicts, so no duplicates will be displayed.
  let startSourceVerdicts = startVerdictsResponse?.sourceVerdicts || [];
  const combinedSourceVerdicts =
    endSourceVerdicts?.concat(startSourceVerdicts) || [];
  endSourceVerdicts = combinedSourceVerdicts.slice(
    0,
    Math.ceil(combinedSourceVerdicts.length / 2),
  );
  startSourceVerdicts = combinedSourceVerdicts.slice(
    Math.ceil(combinedSourceVerdicts.length / 2),
  );

  const expandSegment = () => {
    setSourceVerdictCount(sourceVerdictCount + 15);
    sourceVerdictNumberChanged(sourceVerdictCount);
  };

  const ExpandButton = (
    <Tooltip title="Expand 30 more tests">
      <Button
        variant="contained"
        sx={{
          backgroundColor: 'grey.50',
          display: 'flex',
          width: '32px',
          gap: 0,
          height: '60px',
          '&:hover': {
            backgroundColor: 'grey.100',
          },
        }}
        onClick={expandSegment}
      >
        <MoreHorizIcon sx={{ color: 'text.secondary' }}></MoreHorizIcon>
      </Button>
    </Tooltip>
  );

  return (
    <ButtonGroup
      sx={{
        display: 'flex',
        alignItems: 'center',
        width: '100%',
        gap: '2px',
        pr: '2px',
      }}
    >
      {isFailingSourceVerdict(
        segment.endPosition,
        endSourceVerdicts as SourceVerdict[],
        startSourceVerdicts as SourceVerdict[],
      )
        ? FailSourceVerdictBox(createBlamelistLink(segment.endPosition))
        : PassSourceVerdictBox(createBlamelistLink(segment.endPosition))}
      <Box
        sx={{
          width: '8px',
          height: '72px',
          backgroundColor: 'grey.300',
          display: 'flex',
          clipPath: 'polygon(0% 0%, 100% 5%, 100% 95%, 0% 100%)',
        }}
      ></Box>
      {endSourceVerdicts &&
        endSourceVerdicts
          .slice(0, sourceVerdictCount)
          .map((sourceVerdict) =>
            PassFailSourceVerdictBox(
              sourceVerdict,
              endSourceVerdicts as SourceVerdict[],
              startSourceVerdicts as SourceVerdict[],
              currentTestResultPosition,
              createBlamelistLink(sourceVerdict.position),
            ),
          )}
      {ExpandButton}
      {startSourceVerdicts &&
        startSourceVerdicts
          .slice(-sourceVerdictCount)
          .map((sourceVerdict) =>
            PassFailSourceVerdictBox(
              sourceVerdict,
              endSourceVerdicts as SourceVerdict[],
              startSourceVerdicts as SourceVerdict[],
              currentTestResultPosition,
              createBlamelistLink(sourceVerdict.position),
            ),
          )}
      <Box
        sx={{
          width: '8px',
          height: '72px',
          backgroundColor: 'grey.300',
          display: 'flex',
          clipPath: 'polygon(0% 5%, 100% 0%, 100% 100%, 0% 95%)',
        }}
      ></Box>

      {isFailingSourceVerdict(
        segment.startPosition,
        endSourceVerdicts as SourceVerdict[],
        startSourceVerdicts as SourceVerdict[],
      )
        ? FailSourceVerdictBox(createBlamelistLink(segment.startPosition))
        : PassSourceVerdictBox(createBlamelistLink(segment.startPosition))}
    </ButtonGroup>
  );
}
