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

import { Alert, AlertTitle, Box, Button } from '@mui/material';
import { ReactNode, useEffect } from 'react';

import {
  ROLLBACK_DURATION_WEEK,
  VersionControlIcon,
  useSwitchVersion,
} from '@/common/components/version_control';
import { logging } from '@/common/tools/logging';
import { genFeedbackUrl } from '@/common/tools/utils';

export interface ErrorDisplayProps {
  readonly error: Error;
  /**
   * When specified, render a highlighted instruction section, in addition to
   * the error message.
   */
  readonly instruction?: ReactNode;
  readonly showFileBugButton?: boolean;
  readonly onTryAgain?: () => void;
}

export function ErrorDisplay({
  error,
  instruction,
  showFileBugButton,
  onTryAgain,
}: ErrorDisplayProps) {
  const switchVersion = useSwitchVersion();
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
      {showFileBugButton && (
        <Button
          href={genFeedbackUrl({
            errMsg: error.message,
            stacktrace: error.stack,
          })}
          target="_blank"
          rel="noopener"
          color="secondary"
        >
          File a bug
        </Button>
      )}
      {onTryAgain && <Button onClick={() => onTryAgain()}>Try Again</Button>}
      {UI_VERSION_TYPE === 'new-ui' && (
        <Box sx={{ fontSize: '0.8em', marginTop: '10px' }}>
          If you suspect this is a recent regression in LUCI UI, you can try{' '}
          <Button
            endIcon={<VersionControlIcon />}
            size="small"
            onClick={() => switchVersion()}
            sx={{
              padding: 0,
              fontSize: '0.85em',
              verticalAlign: 'baseline',
              marginRight: '4px',
              '& > .MuiButton-endIcon': {
                marginTop: '-1px',
                marginLeft: '2px',
                '& > .MuiSvgIcon-root': {
                  fontSize: '1.5em',
                },
              },
            }}
          >
            switching
          </Button>{' '}
          to the LUCI UI version released {ROLLBACK_DURATION_WEEK} weeks ago.
        </Box>
      )}
    </Alert>
  );
}
