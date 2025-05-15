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

import BugReportIcon from '@mui/icons-material/BugReport';
import TroubleshootIcon from '@mui/icons-material/Troubleshoot';
import { Box, CircularProgress, Link, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import React, { useMemo } from 'react';

import { useAnalysesClient as useLuciBisectionClient } from '@/common/hooks/prpc_clients';
import {
  getStatusStyle,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';
import {
  AnalysisRunStatus,
  BatchGetTestAnalysesRequest,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus as BisectionAnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useProject, useTestVariant } from '@/test_investigation/context';

import { useAssociatedBugs } from '../context';
import { useTestVariantBranch } from '../context/context';
import {
  NO_ASSOCIATED_BUGS_TEXT,
  BISECTION_NO_ANALYSIS_TEXT,
  BISECTION_DATA_INCOMPLETE_TEXT,
} from '../types';
// Helper to map TestVariantStatus to SemanticStatusType
function getSemanticStatusFromTestVariant(
  tvStatus: TestVariantStatus,
): SemanticStatusType {
  switch (tvStatus) {
    case TestVariantStatus.EXPECTED:
      return 'expected';
    case TestVariantStatus.UNEXPECTED:
      return 'failure';
    case TestVariantStatus.UNEXPECTEDLY_SKIPPED:
      return 'unexpectedly_skipped';
    case TestVariantStatus.FLAKY:
      return 'flaky';
    case TestVariantStatus.EXONERATED:
      return 'exonerated';
    case TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED:
    default:
      return 'unknown';
  }
}

export function OverviewStatusSection() {
  const bisectionClient = useLuciBisectionClient();
  const testVariant = useTestVariant();
  const project = useProject();
  const associatedBugs = useAssociatedBugs();
  const testVariantBranch = useTestVariantBranch();

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

  const mainStatusSemanticType = useMemo(
    () => getSemanticStatusFromTestVariant(testVariant.status),
    [testVariant.status],
  );
  const mainStatusStyle: StatusStyle = useMemo(
    () => getStatusStyle(mainStatusSemanticType),
    [mainStatusSemanticType],
  );
  const MainStatusIconComponent = mainStatusStyle.icon;

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

  // Prepare the display string, e.g., "Failed" instead of "UNEXPECTED"
  const displayStatusString = useMemo(() => {
    const rawStatusString = TestVariantStatus[testVariant.status];
    return rawStatusString
      .replace('UNEXPECTEDLY_', '')
      .replace('UNEXPECTED', 'Failed');
  }, [testVariant.status]);

  return (
    <>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {MainStatusIconComponent && (
          <MainStatusIconComponent
            sx={{
              fontSize: 20,
              color: mainStatusStyle.iconColor || mainStatusStyle.textColor,
            }}
          />
        )}
        <Typography
          variant="body1"
          sx={{ fontWeight: 'bold', color: mainStatusStyle.textColor }}
        >
          {displayStatusString}
        </Typography>
      </Box>
      <Typography variant="body2" color="text.secondary" component="div">
        Related bug(s):{' '}
        {associatedBugs && associatedBugs.length > 0 ? (
          associatedBugs.map((bug, index) => (
            <React.Fragment key={`${bug.system}-${bug.id}`}>
              {index > 0 && <Typography component="span">, </Typography>}
              <Link
                href={bug.url || `https://${bug.system}.com/${bug.id}`}
                target="_blank"
                rel="noopener noreferrer"
                sx={{ display: 'inline-flex', alignItems: 'center' }}
              >
                <BugReportIcon
                  sx={{
                    fontSize: 18,
                    mr: 0.25,
                    color: 'var(--gm3-color-primary)',
                  }}
                />
                {bug.linkText || `${bug.system}/${bug.id}`}
              </Link>
            </React.Fragment>
          ))
        ) : (
          <Typography component="span" color="text.disabled">
            {NO_ASSOCIATED_BUGS_TEXT}
          </Typography>
        )}
      </Typography>

      <Typography variant="body2" color="text.secondary" component="div">
        <Box
          component="span"
          sx={{ display: 'inline-flex', alignItems: 'center' }}
        >
          <TroubleshootIcon
            sx={{
              fontSize: 18,
              mr: 0.5,
              color: 'var(--gm3-color-on-surface-variant)',
            }}
          />{' '}
          Culprit Searching: {bisectionDisplayInfo.textElement}
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
      </Typography>
    </>
  );
}
