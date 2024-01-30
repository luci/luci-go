// Copyright 2023 The LUCI Authors.
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

import { Alert, AlertTitle, Button } from '@mui/material';
import { ReactNode, useEffect } from 'react';

import { logging } from '@/common/tools/logging';
import { genFeedbackUrl } from '@/common/tools/utils';

export interface ErrorDisplayProps {
  readonly error: Error;
  /**
   * When specified, render a highlighted instruction section, in addition to
   * the error message.
   */
  readonly instruction?: ReactNode;
  readonly onTryAgain?: () => void;
}

export function ErrorDisplay({
  error,
  instruction,
  onTryAgain,
}: ErrorDisplayProps) {
  useEffect(() => {
    // Log the error to so we can inspect it in the browser console.
    logging.error(error);
  }, [error]);

  return (
    <Alert severity="error" sx={{ '& > .MuiAlert-message': { flexGrow: 1 } }}>
      <AlertTitle>Error</AlertTitle>
      {instruction && (
        <div css={{ padding: '2px 5px', background: 'var(--warning-color)' }}>
          {instruction}
        </div>
      )}
      <pre
        css={{
          whiteSpace: 'pre-wrap',
          overflowWrap: 'break-word',
        }}
      >
        {error.message}
      </pre>
      <Button
        href={genFeedbackUrl(error.message, error.stack)}
        target="_blank"
        rel="noopener"
      >
        File a bug
      </Button>
      {onTryAgain && <Button onClick={() => onTryAgain()}>Try Again</Button>}
    </Alert>
  );
}
