// Copyright 2024 The LUCI Authors.
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

import { Box } from '@mui/material';
import { useState } from 'react';
import { Outlet } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LabsWarningAlert } from '@/common/components/labs_warning_alert';
import { GenFeedbackUrlArgs } from '@/common/tools/utils';
import { ContentGroup } from '@/generic_libs/components/google_analytics';

export function Component() {
  const [feedbackUrlArgs, setFeedbackUrlArgs] = useState<GenFeedbackUrlArgs>();
  return (
    <>
      <Box sx={{ width: '100%', backgroundColor: 'rgb(229, 246, 253)' }}>
        {/* Ensures the labs warning is rendered even when there's an error.
         **
         ** Some lab pages are discoverable from the production pages. When the
         ** lab page breaks, we don't want users to think this is a production
         ** page. */}
        <LabsWarningAlert
          feedbackUrlArgs={feedbackUrlArgs}
          sx={{
            backgroundColor: 'rgb(229, 246, 253)',
            // In case the page grows wider than 100vw, make
            // <LabsWarningAlert /> sticky so its text content doesn't get
            // scrolled away.
            position: 'sticky',
            boxSizing: 'border-box',
            maxWidth: 'calc(100vw - var(--accumulated-left) - 25px)',
            left: 'calc(var(--accumulated-left))',
          }}
        />
      </Box>
      <ContentGroup group="labs">
        <RecoverableErrorBoundary
          // See the documentation in `<LoginPage />` to learn why we handle
          // error this way.
          key="labs"
        >
          <Outlet context={{ setFeedbackUrlArgs }} />
        </RecoverableErrorBoundary>
      </ContentGroup>
    </>
  );
}
