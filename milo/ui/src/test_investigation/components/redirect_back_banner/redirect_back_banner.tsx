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

import { Feedback as FeedbackIcon } from '@mui/icons-material';
import { Box, Button, Link } from '@mui/material';
import Alert, { AlertProps } from '@mui/material/Alert';
import { useMemo } from 'react';
import { Link as RouterLink } from 'react-router';

import { useFeatureFlag } from '@/common/feature_flags';
import { genFeedbackUrl } from '@/common/tools/utils';
import { OutputTestVerdict } from '@/common/types/verdict';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useProject } from '@/test_investigation/context';
import { NEW_TEST_INVESTIGATION_PAGE_FLAG } from '@/test_investigation/pages/features';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';
import {
  isPresubmitRun,
  getBuildBucketBuildId,
} from '@/test_investigation/utils/test_info_utils';

export interface RedirectBackBannerProps extends AlertProps {
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict | null;
  parsedTestId?: string | null;
  parsedVariantDef?: Readonly<Record<string, string>> | null;
}

export function RedirectBackBanner({
  invocation,
  testVariant,
  parsedTestId,
  parsedVariantDef,
  ...alertProps
}: RedirectBackBannerProps) {
  // This is here purely for the side effect of enabling the opt out dialog on the page.
  useFeatureFlag(NEW_TEST_INVESTIGATION_PAGE_FLAG);

  const { trackEvent } = useGoogleAnalytics();
  const project = useProject();

  const buildId = useMemo(
    () => getBuildBucketBuildId(invocation),
    [invocation],
  );

  const legacyUrl = useMemo(() => {
    if (!buildId) {
      return '';
    }

    const baseUrl = `/ui/b/${buildId}/test-results?view=legacy`;
    let query = '';

    if (testVariant) {
      query = `ID:${encodeURIComponent(
        testVariant.testId,
      )} VHash:${testVariant.variantHash}`;
    } else if (parsedTestId || parsedVariantDef) {
      const queryParts: string[] = [];
      if (parsedTestId) {
        queryParts.push(`ID:${encodeURIComponent(parsedTestId)}`);
      }
      if (parsedVariantDef) {
        Object.entries(parsedVariantDef).forEach(([key, value]) => {
          queryParts.push(
            `V:${encodeURIComponent(key)}=${encodeURIComponent(value)}`,
          );
        });
      }
      query = queryParts.join(' ');
    }

    return query ? `${baseUrl}&q=${query}` : baseUrl;
  }, [buildId, testVariant, parsedTestId, parsedVariantDef]);

  if (!legacyUrl) {
    return null;
  }

  const feedbackBugtemplateComment = `You can use this entry to log an issue or provide a recommendation for the new Test Results Page.

Please include a short description of the issue or suggestion and, if applicable, describe steps to reproduce and attach a screenshot.

From Link: ${self.location.href}`;

  return (
    <Alert
      severity="info"
      action={
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            component={Link}
            target="_blank"
            href={genFeedbackUrl({
              bugComponent: '1838234',
              customComment: feedbackBugtemplateComment,
            })}
            color="primary"
            size="small"
            variant="contained"
            startIcon={<FeedbackIcon />}
            sx={{ textTransform: 'none' }}
          >
            File UI bug
          </Button>
          <Link
            component={RouterLink}
            to={legacyUrl}
            underline="always"
            sx={{ display: 'flex', alignItems: 'center' }}
            onClick={() =>
              trackEvent('user_action', {
                componentName: 'back_to_old_view',
                project,
                invocationType: isPresubmitRun(invocation)
                  ? 'presubmit'
                  : 'postsubmit',
              })
            }
          >
            Go back to old UI
          </Link>
        </Box>
      }
      {...alertProps}
    >
      You are viewing the new test results UI.
    </Alert>
  );
}
