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
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import {
  pairsToPlaceholderDict,
  renderMustacheMarkdown,
  shouldLoadDependencyBuild,
} from '@/common/tools/instruction/instruction_utils';
import { parseInvId } from '@/common/tools/invocation_utils';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BuildsClientImpl,
  GetBuildRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { InstructionDependencyChain_Node } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

const FIELD_MASK = Object.freeze([
  'id',
  'builder',
  'input',
  'output',
  'tags',
] as const);

export interface InstructionDependencyProps {
  readonly dependencyNode: InstructionDependencyChain_Node;
}

export function InstructionDependency({
  dependencyNode,
}: InstructionDependencyProps) {
  const [expanded, setExpanded] = useState(false);
  const match = dependencyNode.instructionName.match(
    /invocations\/([^/]+)\/instructions\/([^/]+)/,
  );
  const instructionID = match ? match[2] : '';
  const invocationID = match ? match[1] : '';
  const buildID = buildIDFromInvocationID(invocationID);
  const displayName = dependencyNode.descriptiveName || instructionID;

  const client = usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
  });
  const { data } = useQuery({
    ...client.GetBuild.query(
      GetBuildRequest.fromPartial({
        id: buildID,
        mask: {
          fields: FIELD_MASK,
        },
      }),
    ),
    enabled:
      shouldLoadDependencyBuild(dependencyNode.content) &&
      buildID !== '' &&
      expanded,
  });
  const placeholderData = instructionPlaceHolderData(data);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expanded) => setExpanded(expanded)}>
        Dependency: {displayName}
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
              html={renderMustacheMarkdown(
                dependencyNode.content,
                placeholderData,
              )}
            />
          </Typography>
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}

function instructionPlaceHolderData(build: Build | undefined) {
  if (build === undefined) {
    return {};
  }
  const tagsDict = pairsToPlaceholderDict(build.tags);

  const result = {
    build: {
      id: build.id,
      builder: {
        project: build.builder?.project,
        bucket: build.builder?.bucket,
        builder: build.builder?.builder,
      },
      tags: tagsDict,
      inputProperties: build.input?.properties || {},
      outputProperties: build.output?.properties || {},
    },
  };
  return result;
}

function buildIDFromInvocationID(invID: string): string {
  const parsedInvId = parseInvId(invID);
  if (parsedInvId.type !== 'build') {
    return '';
  }
  return parsedInvId.buildId;
}
