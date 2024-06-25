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

import { AlertTitle, Typography } from '@mui/material';
import Alert from '@mui/material/Alert';
import { useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { InstructionDependencyChain_Node } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

export interface InstructionDependencyProps {
  readonly dependencyNode: InstructionDependencyChain_Node;
}

export function InstructionDependency({
  dependencyNode,
}: InstructionDependencyProps) {
  const [expanded, setExpanded] = useState(false);
  // TODO: Perhaps it's better to introduce "descriptive name" for instruction,
  // instead of using instruction ID.
  const instructionId = dependencyNode.instructionName.match(
    /invocations\/[^/]+\/instructions\/([^/]+)/,
  )?.[1];
  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expanded) => setExpanded(expanded)}>
        Dependency: {instructionId}
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {dependencyNode.error && (
          <Alert severity="error">
            <AlertTitle>Error loading dependency</AlertTitle>
            {dependencyNode.error}
          </Alert>
        )}
        {dependencyNode.content && (
          <Typography
            component="span"
            sx={{ color: 'var(--default-text-color)' }}
          >
            <SanitizedHtml
              html={resolveMustacheMarkdown(dependencyNode.content)}
            />
          </Typography>
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}

function resolveMustacheMarkdown(content: string | undefined): string {
  if (content === undefined) {
    return '';
  }
  // TODO: Get query buildbucket data for placeholder data.
  return renderMarkdown(content || '');
}
