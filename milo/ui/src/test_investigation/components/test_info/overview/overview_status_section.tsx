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

import InfoOutlineIcon from '@mui/icons-material/InfoOutlined';
import {
  Box,
  CircularProgress,
  IconButton,
  Link,
  Tooltip,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import React, { useMemo } from 'react';

import { BugCard } from '@/common/components/bug_card';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { VerdictStatusIcon } from '@/common/components/verdict_status_icon';
import { useAnalysesClient as useLuciBisectionClient } from '@/common/hooks/prpc_clients';
import { exonerationReasonToJSON } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  AnalysisRunStatus,
  BatchGetTestAnalysesRequest,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus as BisectionAnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { useProject, useTestVariant } from '@/test_investigation/context';
import { useInvocation } from '@/test_investigation/context';
import { useDisplayStatusString } from '@/test_investigation/context/context';
import { isPresubmitRun } from '@/test_investigation/utils/test_info_utils';

import {
  NO_ASSOCIATED_BUGS_TEXT,
  BISECTION_NO_ANALYSIS_TEXT,
  BISECTION_DATA_INCOMPLETE_TEXT,
} from '../constants';
import { useAssociatedBugs, useTestVariantBranch } from '../context';

interface OverviewStatusSectionProps {
  expanded: boolean;
}

export function OverviewStatusSection({
  expanded,
}: OverviewStatusSectionProps) {
  const invocation = useInvocation();
  const bisectionClient = useLuciBisectionClient();
  const testVariant = useTestVariant();
  const project = useProject();
  const associatedBugs = useAssociatedBugs();
  const testVariantBranch = useTestVariantBranch();
  const displayStatusString = useDisplayStatusString();

  const bisectionAnalysisQueryEnabled = !!(
    project &&
    testVariant?.testId &&
    testVariant?.variantHash &&
    testVariantBranch?.refHash
  );

  const bisectionTestFailureIdentifier = useMemo(() => {
    if (!bisectionAnalysisQueryEnabled) return undefined;
    return {
      testId: testVariant!.testId,
      variantHash: testVariant!.variantHash!,
      refHash: testVariantBranch!.refHash!,
    };
  }, [bisectionAnalysisQueryEnabled, testVariant, testVariantBranch]);

  const bisectionAnalysisRequest = useMemo(() => {
    if (!bisectionAnalysisQueryEnabled || !bisectionTestFailureIdentifier) {
      return BatchGetTestAnalysesRequest.fromPartial({});
    }
    return BatchGetTestAnalysesRequest.fromPartial({
      project: project!,
      testFailures: [bisectionTestFailureIdentifier],
    });
  }, [bisectionAnalysisQueryEnabled, project, bisectionTestFailureIdentifier]);

  const { data: bisectionAnalysis, isLoading: isLoadingBisectionAnalysis } =
    useQuery({
      ...bisectionClient.BatchGetTestAnalyses.query(bisectionAnalysisRequest),
      enabled: bisectionAnalysisQueryEnabled,
      staleTime: 15 * 60 * 1000, // More stale as bisection results change less often
      select: (response) => {
        return response.testAnalyses && response.testAnalyses.length > 0
          ? response.testAnalyses[0]
          : null;
      },
    });

  // Bisection display logic (text and link)
  const bisectionDisplayInfo = useMemo(() => {
    if (
      !bisectionAnalysis ||
      !bisectionAnalysis.analysisId ||
      bisectionAnalysis.analysisId.toString() === '0'
    ) {
      return {
        textElement: (
          <Typography component="span" color="text.disabled">
            {BISECTION_NO_ANALYSIS_TEXT}
          </Typography>
        ),
        link: undefined,
      };
    }
    const status = bisectionAnalysis.status;
    const runStatus = bisectionAnalysis.runStatus;
    const statusText =
      BisectionAnalysisStatus[status] || `STATUS_CODE_${status}`;
    const runStatusText =
      AnalysisRunStatus[runStatus] || `RUN_STATUS_CODE_${runStatus}`;
    const bisectionLink =
      project &&
      bisectionAnalysis.analysisId &&
      bisectionAnalysis.analysisId.toString() !== '0'
        ? `https://ci.chromium.org/ui/p/${project}/bisection/test-analysis/b/${bisectionAnalysis.analysisId}`
        : undefined;

    let textElement = <></>;
    if (status === BisectionAnalysisStatus.FOUND && bisectionAnalysis.culprit) {
      const culprit = bisectionAnalysis.culprit;
      const culpritCommit = culprit.commit;
      textElement = (
        <>
          Culprit:{' '}
          <Link
            href={culprit.reviewUrl}
            target="_blank"
            rel="noopener noreferrer"
          >
            {culprit.reviewTitle || culpritCommit?.position || 'details'}
          </Link>
          {culpritCommit?.position && ` (${culpritCommit.position})`}
        </>
      );
    } else {
      let displayText = `Status: ${statusText}`;
      if (
        status === BisectionAnalysisStatus.RUNNING ||
        runStatus === AnalysisRunStatus.STARTED
      ) {
        displayText = `Bisection in progress (${runStatusText})...`;
      } else if (status === BisectionAnalysisStatus.NOTFOUND) {
        displayText = 'Culprit not found by Bisection.';
      } else if (status === BisectionAnalysisStatus.ERROR) {
        displayText = 'Bisection analysis error.';
      } else if (
        status === BisectionAnalysisStatus.SUSPECTFOUND &&
        bisectionAnalysis.nthSectionResult?.suspect?.commit
      ) {
        const suspectCommit = bisectionAnalysis.nthSectionResult.suspect.commit;
        displayText = `Suspect found: ${suspectCommit?.position || 'details'}`;
      } else if (
        status === BisectionAnalysisStatus.ANALYSIS_STATUS_UNSPECIFIED ||
        runStatus === AnalysisRunStatus.ANALYSIS_RUN_STATUS_UNSPECIFIED
      ) {
        textElement = (
          <Typography component="span" color="text.disabled">
            {BISECTION_DATA_INCOMPLETE_TEXT}
          </Typography>
        );
      } else {
        textElement = <Typography component="span">{displayText}</Typography>;
      }
      if (!textElement && displayText) {
        // Ensure textElement is assigned if not by special cases
        textElement = <Typography component="span">{displayText}</Typography>;
      }
    }
    return { textElement, link: bisectionLink };
  }, [bisectionAnalysis, project]);

  let exonerationExplanation = '';
  let exonerationReason = '';
  if (
    testVariant.statusOverride === TestVerdict_StatusOverride.EXONERATED &&
    testVariant.exonerations[0]
  ) {
    exonerationExplanation = testVariant.exonerations[0].explanationHtml;
    exonerationReason = exonerationReasonToJSON(
      testVariant.exonerations[0].reason,
    );
  }

  return (
    <>
      <Box
        sx={{
          display: 'flex',
          flexDirection: expanded ? 'column' : 'row',
          gap: expanded ? 0 : '80px',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: 1,
          }}
        >
          <Typography variant="body2" color="text.secondary">
            Test Result
          </Typography>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <VerdictStatusIcon
              statusV2={testVariant.statusV2}
              statusOverride={testVariant.statusOverride}
            />
            <Typography
              variant="body1"
              sx={{ fontWeight: '400', fontSize: '24px' }}
            >
              {displayStatusString}
            </Typography>
            {(exonerationReason || exonerationExplanation) && (
              <Tooltip
                title={
                  <Box
                    sx={{ display: 'flex', flexDirection: 'column', gap: 0 }}
                  >
                    {exonerationExplanation && (
                      <Typography variant="caption">
                        Explanation: {exonerationExplanation}
                      </Typography>
                    )}
                    {exonerationReason && (
                      <Typography variant="caption">
                        Reason: {exonerationReason}
                      </Typography>
                    )}
                  </Box>
                }
              >
                <IconButton sx={{ p: 0 }}>
                  <InfoOutlineIcon />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        </Box>
        <Box
          sx={{
            mt: expanded ? '16px' : 0,
            display: 'flex',
            flexDirection: 'row',
            gap: 3,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              flexGrow: 1,
            }}
          >
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ mb: expanded ? 1 : 2 }}
            >
              Related bug(s)
            </Typography>
            {associatedBugs && associatedBugs.length > 0 ? (
              associatedBugs.map((bug) => (
                <React.Fragment key={bug.id}>
                  <HtmlTooltip
                    title={
                      project && (
                        <BugCard
                          project={project}
                          bugId={bug.id}
                          // TODO: Add cluster id here when we have it on the page.
                        />
                      )
                    }
                  >
                    <Box
                      sx={{
                        padding: '2px 8px 2px 8px',
                        borderRadius: '4px',
                        backgroundColor: '#E8F0FE',
                        width: '85px',
                        mb: 0.5,
                      }}
                    >
                      <Link
                        href={bug.url || `https://${bug.system}.com/${bug.id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        variant="caption"
                        color="#3C4043"
                        sx={{ textDecoration: 'none' }}
                      >
                        {bug.linkText || `${bug.system}/${bug.id}`}
                      </Link>
                    </Box>
                  </HtmlTooltip>
                </React.Fragment>
              ))
            ) : (
              <Typography component="span" color="text.disabled">
                {NO_ASSOCIATED_BUGS_TEXT}
              </Typography>
            )}
          </Box>
          {!isPresubmitRun(invocation) && (
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                flexGrow: 1,
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: expanded ? 0 : '16px' }}
              >
                Culprit Searching
              </Typography>
              <Typography>{bisectionDisplayInfo.textElement}</Typography>
              {isLoadingBisectionAnalysis && <CircularProgress />}
              {bisectionDisplayInfo.link && (
                <Link
                  href={bisectionDisplayInfo.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="caption"
                  sx={{ ml: 0.5 }}
                >
                  (View Bisection)
                </Link>
              )}
            </Box>
          )}
        </Box>
      </Box>
    </>
  );
}
