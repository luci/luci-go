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

import { FormControlLabel, Switch, Typography } from '@mui/material';
import { useState } from 'react';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { useInvocation } from '@/test_investigation/context';
import { isAnTSInvocation } from '@/test_investigation/utils/test_info_utils';

import { ArtifactContentHeader } from './artifact_content_header';
import { ArtifactContentSearchMenu } from './artifact_content_search_menu';
import { TextDiffArtifactView } from './text_diff_artifact_view';
import { VirtualizedArtifactContent } from './virtualized_artifact_content';

interface TextDiffArtifactContentProps {
  artifact: Artifact;
  content: string;
  isFullLoading?: boolean;
}

export function TextDiffArtifactContent({
  artifact,
  content,
  isFullLoading,
}: TextDiffArtifactContentProps) {
  const [showRawContentView, setShowRawContentView] = useState(false);

  const invocation = useInvocation();

  const isAnTS = isAnTSInvocation(invocation);

  const actions = (
    <FormControlLabel
      control={
        <Switch
          checked={!showRawContentView}
          onChange={() => setShowRawContentView(!showRawContentView)}
          size="small"
        />
      }
      label="Formatted"
    />
  );

  return (
    <>
      {showRawContentView ? (
        <VirtualizedArtifactContent
          content={content}
          isFullLoading={isFullLoading}
          artifact={artifact}
          hasPassingResults={false}
          isLogComparisonPossible={false}
          showLogComparison={false}
          onToggleLogComparison={() => {}}
          actions={actions}
        />
      ) : (
        <>
          <ArtifactContentHeader
            artifact={artifact}
            contentData={{
              contentType: artifact.contentType || 'text/x-diff',
            }}
            actions={
              <ArtifactContentSearchMenu
                artifact={artifact}
                isLogComparisonPossible={false}
                showLogComparison={false}
                hasPassingResults={false}
                onToggleLogComparison={() => {}}
                isAnTS={isAnTS}
                invocation={invocation as Invocation}
                actions={actions}
              />
            }
          />
          {content ? (
            <TextDiffArtifactView content={content} />
          ) : (
            <Typography sx={{ mt: 1, px: 2 }} color="text.secondary">
              Text artifact is empty.
            </Typography>
          )}
        </>
      )}
    </>
  );
}
