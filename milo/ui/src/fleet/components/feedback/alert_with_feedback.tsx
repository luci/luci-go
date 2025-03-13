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

import { Alert, AlertColor, AlertTitle, Link } from '@mui/material';

import { genFeedbackUrl } from '@/common/tools/utils';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';

interface AlertWithFeedbackProps {
  severity?: AlertColor;
  title?: string;
  children?: React.ReactNode;
  bugErrorMessage?: string;
  testId?: string;
}

export default function AlertWithFeedback({
  severity,
  title,
  children,
  bugErrorMessage,
  testId,
}: AlertWithFeedbackProps) {
  return (
    <Alert severity={severity || 'error'} data-testid={testId}>
      <AlertTitle>{title || 'Oops! An error occured.'}</AlertTitle>
      {children || <p>Oh no! Something didn&apos;t go as planned.</p>}
      <p>
        If you think that shoulnd&apos;t happen, let us know, we will try our
        best to fix that! Just send us your{' '}
        <Link
          href={genFeedbackUrl({
            bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
            errMsg: bugErrorMessage,
          })}
          target="_blank"
        >
          feedback
        </Link>
        !
      </p>
    </Alert>
  );
}
