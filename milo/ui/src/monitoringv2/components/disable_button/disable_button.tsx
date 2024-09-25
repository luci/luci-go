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

import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Alert, Button } from '@mui/material';
import { useState } from 'react';

import { Bug } from '@/monitoringv2/util/server_json';

interface DisableTestButtonProps {
  failureBBID: string;
  testID: string;
  bug?: Bug;
}

export const DisableTestButton = ({
  bug,
  failureBBID,
  testID,
}: DisableTestButtonProps) => {
  const [wasCopied, setWasCopied] = useState(false);
  const [error, setError] = useState('');
  const handleClick = () => {
    let command = 'tools/disable_tests/disable';
    if (bug) {
      // Add the bug ID if present. Only do it if there's exactly one. If there
      // are more than one we don't know which one to use.
      command += ' -b ' + bug.number;
    }

    command += ` ${failureBBID} '${testID}'`;

    navigator.clipboard.writeText(command).catch(function (err) {
      setError(err);
    });

    setWasCopied(true);
    setTimeout(() => setWasCopied(false), 1500);
  };
  return (
    <>
      <Button
        size="small"
        id="copy-disable-command-button"
        title="Copy a command to run from the root of a chromium/src checkout to disable this test."
        onClick={handleClick}
        startIcon={wasCopied ? <CheckIcon /> : <ContentCopyIcon />}
      >
        {wasCopied ? 'Copied' : 'Disable'}
      </Button>
      {error ? (
        <Alert severity="error" onClose={() => setError('')}>
          {error}
        </Alert>
      ) : null}
    </>
  );
};
