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

import { Box, Link } from '@mui/material';
import Alert, { AlertProps } from '@mui/material/Alert';

import { getAntsInvocationId } from '@/common/tools/url_utils';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useProject } from '@/test_investigation/context';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { FileUIBugButton } from './file_ui_bug_button';

export interface RedirectATIBannerProps extends AlertProps {
  invocation: AnyInvocation;
}

export function RedirectATIBanner({
  invocation,
  ...alertProps
}: RedirectATIBannerProps) {
  const { trackEvent } = useGoogleAnalytics();
  const project = useProject();

  // TODO(b/463488897): When on TI page, should link directly to Android test result page instead of just invocation.
  const ATIUrl =
    `https://android-build.corp.google.com/test_investigate/invocation/` +
    getAntsInvocationId(invocation) +
    '?preventRedirect=true';

  return (
    <Alert
      severity="info"
      action={
        <Box sx={{ display: 'flex', gap: 1 }}>
          <FileUIBugButton></FileUIBugButton>
          <Link
            target="_blank"
            href={ATIUrl}
            underline="always"
            sx={{ display: 'flex', alignItems: 'center' }}
            onClick={() =>
              trackEvent('user_action', {
                componentName: 'back_to_ati_view',
                project,
              })
            }
          >
            Go back to ATI
          </Link>
        </Box>
      }
      {...alertProps}
    >
      You are viewing the new test results UI for Android.
    </Alert>
  );
}
