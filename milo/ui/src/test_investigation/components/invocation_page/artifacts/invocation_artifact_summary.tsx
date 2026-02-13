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

import { Box, Typography } from '@mui/material';
import { useMemo } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { useArtifacts } from '@/test_investigation/components/common/artifacts/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

export function InvocationArtifactSummary() {
  const { selectedNode, invocation } = useArtifacts();

  const summaryHtml = useMemo(() => {
    if (
      selectedNode?.isSummary &&
      isRootInvocation(invocation) &&
      invocation.summaryMarkdown
    ) {
      return renderMarkdown(invocation.summaryMarkdown);
    }
    return null;
  }, [selectedNode, invocation]);

  if (!summaryHtml) {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
        }}
      >
        <Typography variant="body1">No summary available.</Typography>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        p: 2,
        '& pre': {
          whiteSpace: 'pre-wrap',
          overflowWrap: 'break-word',
          fontSize: '12px',
        },
        '& *': {
          marginBlock: '10px',
        },
      }}
    >
      <SanitizedHtml html={summaryHtml} />
    </Box>
  );
}
