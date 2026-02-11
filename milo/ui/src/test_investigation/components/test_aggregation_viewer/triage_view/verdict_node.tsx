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

import { Box, Chip, Link, Tooltip, Typography } from '@mui/material';
import { useMemo } from 'react';
import { Link as RouterLink } from 'react-router';

import { generateTestInvestigateUrl } from '@/common/tools/url_utils/url_utils';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVerdict,
  TestVerdict_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { useInvocation } from '@/test_investigation/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';
import { normalizeDrawerFailureReason } from '@/test_investigation/utils/test_variant_utils';

import { getVariantDefinitionString, getVerdictNodeId } from '../context/utils';

import { useTriageViewContext } from './context';

export interface VerdictNodeProps {
  verdict: TestVerdict;
  currentFailureReason?: string;
}

export function VerdictNode({
  verdict,
  currentFailureReason,
}: VerdictNodeProps) {
  const { scrollRequest } = useTriageViewContext();
  const invocation = useInvocation();
  const invocationName = isRootInvocation(invocation) ? invocation.name : '';

  const testId = verdict.testId;
  const nodeId = getVerdictNodeId(verdict);
  const isSelected = scrollRequest?.id === nodeId;

  // Breadcrumbs
  let crumbs = '';
  let label = testId;

  if (verdict.testIdStructured) {
    const { moduleName, coarseName, fineName, caseName, moduleVariant } =
      verdict.testIdStructured;
    if (caseName) {
      if (!moduleName && !coarseName && !fineName) {
        crumbs = '';
      } else {
        const parts = [moduleName];
        const variantText = getVariantDefinitionString(moduleVariant?.def);
        if (variantText) {
          parts.push(`(${variantText})`);
        }
        parts.push(coarseName, fineName);
        crumbs = parts.filter(Boolean).join(' > ');
      }
      label = caseName;
    }
  }

  // Strip "invocations/" or "rootInvocations/"
  const cleanInvocationId = invocationName.replace(
    /^(?:.*\/)?(?:rootInvocations|invocations)\//,
    '',
  );

  const url =
    verdict.testIdStructured && cleanInvocationId
      ? generateTestInvestigateUrl(cleanInvocationId, verdict.testIdStructured)
      : '';

  const otherReasonsCount = useMemo(() => {
    if (!currentFailureReason) return 0;
    const reasons = new Set<string>();

    if (verdict.status === TestVerdict_Status.PASSED) {
      reasons.add('Passed');
    } else if (verdict.status === TestVerdict_Status.SKIPPED) {
      reasons.add('Skipped');
    } else {
      let hasFailures = false;
      verdict.results?.forEach((r) => {
        const status = r.statusV2 || r.status;
        if (
          status === TestResult_Status.FAILED ||
          status === TestResult_Status.EXECUTION_ERRORED
        ) {
          hasFailures = true;
          if (r.failureReason?.primaryErrorMessage) {
            reasons.add(
              normalizeDrawerFailureReason(r.failureReason.primaryErrorMessage),
            );
          } else {
            reasons.add('No failure reason');
          }
        }
      });
      if (!hasFailures) {
        reasons.add('No failure reason');
      }
    }

    const verdictStatusStr = TestVerdict_Status[verdict.status];
    const groupIds = new Set(
      Array.from(reasons).map((r) => `${verdictStatusStr}|${r}`),
    );

    groupIds.delete(currentFailureReason);
    return groupIds.size;
  }, [verdict, currentFailureReason]);

  return (
    <Box
      id={`triage-node-${testId}`}
      sx={{
        pl: 6,
        pr: 2,
        height: '100%',
        display: 'flex',
        flexDirection: 'column', // Stack
        justifyContent: 'center',
        borderLeft: isSelected ? '4px solid #1976d2' : '4px solid transparent',
        bgcolor: isSelected ? 'action.selected' : 'transparent',
        '&:hover': {
          bgcolor: 'action.hover',
        },
      }}
    >
      <Box sx={{ width: '100%', overflow: 'hidden' }}>
        {/* First Line: Test Name and Chip */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Tooltip title={label} placement="bottom-start" enterDelay={500}>
            {url ? (
              <Link
                component={RouterLink}
                to={url}
                underline="hover"
                sx={{
                  typography: 'body2',
                  fontWeight: 'medium',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {label}
              </Link>
            ) : (
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 'medium',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {label}
              </Typography>
            )}
          </Tooltip>
          {/* Update the tooltip to have exactly what the user asked for */}
          {otherReasonsCount > 0 && (
            <Tooltip
              title={`Found in ${otherReasonsCount} more failure reasons`}
            >
              <Chip
                label={`+${otherReasonsCount}`}
                size="small"
                color="default"
                sx={{ height: 20, fontSize: '0.75rem', cursor: 'help' }}
              />
            </Tooltip>
          )}
        </Box>

        {/* Second Line: Breadcrumbs */}
        {crumbs && (
          <Tooltip title={crumbs} placement="bottom-start" enterDelay={500}>
            <Typography
              variant="caption"
              color="text.secondary"
              noWrap
              sx={{ display: 'block', mt: 0 }}
            >
              {crumbs}
            </Typography>
          </Tooltip>
        )}
      </Box>
    </Box>
  );
}
