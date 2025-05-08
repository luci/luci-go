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
import { Box, Link, Typography } from '@mui/material';
import React, { useMemo } from 'react';

import {
  getStatusStyle,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';
import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  TestAnalysis,
  AnalysisRunStatus,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus as BisectionAnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  NO_ASSOCIATED_BUGS_TEXT,
  BISECTION_NO_ANALYSIS_TEXT,
  BISECTION_DATA_INCOMPLETE_TEXT,
} from './types';

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

interface OverviewStatusSectionProps {
  testVariantStatus: TestVariantStatus;
  associatedBugs?: readonly AssociatedBug[];
  bisectionAnalysis?: TestAnalysis | null;
  project?: string;
}

export function OverviewStatusSection({
  testVariantStatus,
  associatedBugs,
  bisectionAnalysis,
  project,
}: OverviewStatusSectionProps): JSX.Element {
  const mainStatusSemanticType = useMemo(
    () => getSemanticStatusFromTestVariant(testVariantStatus),
    [testVariantStatus],
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

    let textElement: JSX.Element = <></>;
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
    const rawStatusString = TestVariantStatus[testVariantStatus];
    return rawStatusString
      .replace('UNEXPECTEDLY_', '')
      .replace('UNEXPECTED', 'Failed');
  }, [testVariantStatus]);

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
