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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Link,
  Typography,
} from '@mui/material';
import { useMemo } from 'react';
import { useParams } from 'react-router';

import { TestResultSummary } from '@/common/components/test_result_summary';
import {
  getStatusStyle,
  semanticStatusForTestVariant,
} from '@/common/styles/status_styles';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import {
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { getTestVariantURL } from '@/test_investigation/utils/test_info_utils';

export interface FailureSummaryProps {
  testVariant: TestVariant;
  testTextToCopy: string;
  rowLabel: string;
  isLegacyInvocation: boolean;
}

const statusPriorityMap = {
  [TestResult_Status.FAILED]: 0,
  [TestResult_Status.SKIPPED]: 1,
  [TestResult_Status.EXECUTION_ERRORED]: 2,
  [TestResult_Status.PASSED]: 3,
  [TestResult_Status.PRECLUDED]: 4,
  [TestResult_Status.STATUS_UNSPECIFIED]: 5,
};

export function FailureSummary({
  testVariant,
  testTextToCopy,
  rowLabel,
  isLegacyInvocation,
}: FailureSummaryProps) {
  const { invocationId } = useParams<{ invocationId: string }>();
  if (!invocationId) {
    throw new Error('Invocation ID is required');
  }

  const resultToDisplay: TestResult = useMemo(() => {
    const resultsBundle = testVariant.results;
    return resultsBundle
      .filter((resultsBundle) => resultsBundle.result !== undefined)
      .map((resultsBundle) => resultsBundle.result!)
      .sort((a, b) => {
        return statusPriorityMap[a.statusV2] - statusPriorityMap[b.statusV2];
      })[0];
  }, [testVariant.results]);

  let failureSummaryText = '';
  if (!resultToDisplay.summaryHtml) {
    failureSummaryText =
      resultToDisplay.failureReason?.primaryErrorMessage ||
      resultToDisplay.failureReason?.errors?.[0]?.message ||
      resultToDisplay?.skippedReason?.reasonMessage ||
      '';
  }

  const styles = useMemo(() => {
    const semanticStatus = semanticStatusForTestVariant(testVariant);
    return getStatusStyle(semanticStatus);
  }, [testVariant]);
  const IconComponent = styles.icon;

  return (
    <>
      {failureSummaryText || resultToDisplay.summaryHtml ? (
        <Accordion
          sx={{
            p: 0,
            border: 'none',
            backgroundColor: 'rgba(0, 0, 255, 0)',
          }}
        >
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            sx={{
              p: 0,
              flexDirection: 'row-reverse',
            }}
          >
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                p: 0,
              }}
            >
              {IconComponent && (
                <IconComponent
                  sx={{
                    fontSize: '18px',
                    color: styles.iconColor,
                  }}
                />
              )}
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  maxWidth: '82vw',
                }}
              >
                <Link
                  href={getTestVariantURL(
                    invocationId,
                    testVariant,
                    isLegacyInvocation,
                  )}
                  variant="body2"
                  sx={{
                    textAlign: 'left',
                    textTransform: 'none',
                    textDecoration: 'none',
                    color: styles.textColor,
                  }}
                  onClick={(e) => {
                    {
                      e.stopPropagation();
                    }
                  }}
                >
                  {rowLabel}
                </Link>
                <CopyToClipboard
                  textToCopy={testTextToCopy}
                  aria-label="Copy text."
                  sx={{ ml: 0.5, minWidth: '18px' }}
                />
                <Typography
                  variant="caption"
                  sx={{
                    p: 1,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    display: '-webkit-box',
                    WebkitLineClamp: '1',
                    whiteSpace: 'nowrap',
                    WebkitBoxOrient: 'vertical',
                    color: 'text.secondary',
                  }}
                >
                  {failureSummaryText}
                </Typography>
              </Box>
            </Box>
          </AccordionSummary>
          <AccordionDetails sx={{ p: 0 }}>
            {resultToDisplay.summaryHtml ? (
              <TestResultSummary testResult={resultToDisplay} />
            ) : (
              <Box sx={{ maxWidth: '90vw' }}>
                {(resultToDisplay.failureReason?.primaryErrorMessage ||
                  resultToDisplay.failureReason?.errors?.[0]?.message ||
                  resultToDisplay?.skippedReason?.reasonMessage) && (
                  <Typography
                    sx={{
                      p: 1,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      display: '-webkit-box',
                      WebkitLineClamp: '6',
                      WebkitBoxOrient: 'vertical',
                    }}
                    variant="caption"
                  >
                    {resultToDisplay.failureReason?.primaryErrorMessage ||
                      resultToDisplay.failureReason?.errors?.[0]?.message ||
                      resultToDisplay?.skippedReason?.reasonMessage}
                  </Typography>
                )}
              </Box>
            )}
          </AccordionDetails>
        </Accordion>
      ) : (
        <>
          <Link
            href={getTestVariantURL(
              invocationId,
              testVariant,
              isLegacyInvocation,
            )}
            variant="body2"
            sx={{
              textAlign: 'left',
              textTransform: 'none',
              textDecoration: 'none',
              color: styles.textColor,
            }}
            onClick={(e) => {
              {
                e.stopPropagation();
              }
            }}
          >
            {rowLabel}
          </Link>
          <CopyToClipboard
            textToCopy={testTextToCopy}
            aria-label="Copy text."
            sx={{ ml: 0.5 }}
          />
        </>
      )}
    </>
  );
}
