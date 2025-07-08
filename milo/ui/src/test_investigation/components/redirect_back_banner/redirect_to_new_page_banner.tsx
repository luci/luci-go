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

import { Button } from '@mui/material';
import Alert, { AlertProps } from '@mui/material/Alert';
import { useCallback } from 'react';

import { useFeatureFlag, useSetFeatureFlag } from '@/common/feature_flags';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { NEW_TEST_INVESTIGATION_PAGE_FLAG } from '@/test_investigation/pages/features';

export interface RedirectToNewPageBannerProps extends AlertProps {
  project: string;
}

export function RedirectToNewPageBanner({
  project,
  ...alertProps
}: RedirectToNewPageBannerProps) {
  const featureFlag = useFeatureFlag(NEW_TEST_INVESTIGATION_PAGE_FLAG);
  const setFeatureFlag = useSetFeatureFlag(NEW_TEST_INVESTIGATION_PAGE_FLAG);
  const { trackEvent } = useGoogleAnalytics();

  // TODO: Track all feature flag opt in/outs generically, rather than only from this call site.
  const trackOptChange = useCallback(
    (value: boolean) => {
      trackEvent(value ? 'feature_opt_in' : 'feature_opt_out', {
        project,
        featureFlag: NEW_TEST_INVESTIGATION_PAGE_FLAG.config.name,
      });
    },
    [project, trackEvent],
  );

  return (
    <Alert
      severity="info"
      action={
        featureFlag ? (
          <Button
            size="small"
            variant="contained"
            onClick={() => {
              trackOptChange(false);
              setFeatureFlag(false);
            }}
          >
            Opt out of the new UI
          </Button>
        ) : (
          <Button
            size="small"
            variant="contained"
            onClick={() => {
              trackOptChange(true);
              setFeatureFlag(true);
            }}
          >
            Opt in to the new UI
          </Button>
        )
      }
      {...alertProps}
    >
      {featureFlag ? (
        <>
          You are temporarily viewing the deprecated test results page. If you
          wish to permanently use this view, please opt out.
        </>
      ) : (
        <>
          This test results view is deprecated. Please opt in to the new test
          results view and let us know any feedback.
        </>
      )}
    </Alert>
  );
}
