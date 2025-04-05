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

import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Button, Paper, Tooltip } from '@mui/material';
import { Box } from '@mui/system';

interface CodeSnippetProps {
  copyText: string;
  displayText?: string;
}

export default function CodeSnippet({
  copyText,
  displayText,
}: CodeSnippetProps) {
  const handleCopy = async () => {
    await navigator.clipboard.writeText(copyText);
  };

  return (
    <Paper
      elevation={0}
      variant="outlined"
      css={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        flexDirection: 'row',
      }}
    >
      <Box sx={{ p: '20px' }}>
        <code>{displayText ?? copyText}</code>
      </Box>
      <Box sx={{ flexGrow: 1 }} />
      <Box css={{ marginRight: '8px' }}>
        <Tooltip title="Copy to clipboard">
          <Button
            onClick={handleCopy}
            sx={{ minWidth: '30px' }}
            aria-label="Copy to clipboard"
          >
            <ContentCopyIcon sx={{ height: '20px' }} />
          </Button>
        </Tooltip>
      </Box>
    </Paper>
  );
}
