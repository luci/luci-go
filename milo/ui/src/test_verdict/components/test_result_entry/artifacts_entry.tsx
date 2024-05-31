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

import { Alert, AlertTitle, Link, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';

import { getInvURLPath } from '@/common/tools/url_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { ArtifactLink } from '@/test_verdict/components/artifact_link';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { parseTestResultName } from '@/test_verdict/tools/utils';

export interface ArtifactsEntryProps {
  readonly testResultName: string;
}

export function ArtifactsEntry({ testResultName }: ArtifactsEntryProps) {
  const [expanded, setExpanded] = useState(false);

  const client = useResultDbClient();
  const resultArtifactsQuery = useQuery(
    client.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({ parent: testResultName }),
    ),
  );
  const parsedTestResultName = parseTestResultName(testResultName);
  const invArtifactsQuery = useQuery(
    client.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: 'invocations/' + parsedTestResultName.invocationId,
      }),
    ),
  );

  const isLoading =
    resultArtifactsQuery.isLoading || invArtifactsQuery.isLoading;
  const artifactsCount =
    (resultArtifactsQuery.data?.artifacts.length || 0) +
    (invArtifactsQuery.data?.artifacts.length || 0);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expanded) => setExpanded(expanded)}>
        Artifacts:{' '}
        {isLoading ? (
          <DotSpinner />
        ) : (
          <Typography
            component="span"
            sx={{ color: 'var(--greyed-out-text-color)' }}
          >
            {artifactsCount}
          </Typography>
        )}
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {isLoading && <DotSpinner />}
        {
          // Artifacts are optional resources. Users may not have access to the
          // artifacts. Handle the error locally so the error display do not
          // affect parent's layout.
          resultArtifactsQuery.isError && (
            <Alert severity="error">
              <AlertTitle>Failed to query result artifacts</AlertTitle>
              {`${resultArtifactsQuery.error}`}
            </Alert>
          )
        }
        {resultArtifactsQuery.data?.artifacts.length ? (
          <ul>
            {resultArtifactsQuery.data?.artifacts.map((artifact) => (
              <li key={artifact.name}>
                <ArtifactLink artifact={artifact} />
              </li>
            ))}
          </ul>
        ) : (
          <></>
        )}
        {
          // Artifacts are optional resources. Users may not have access to the
          // artifacts. Handle the error locally so the error display do not
          // affect parent's layout.
          invArtifactsQuery.isError && (
            <Alert severity="error">
              <AlertTitle>Failed to query invocation artifacts</AlertTitle>
              {`${invArtifactsQuery.error}`}
            </Alert>
          )
        }
        {invArtifactsQuery.data?.artifacts.length ? (
          <>
            <div>
              From the parent invocation{' '}
              <Link
                component={RouterLink}
                to={getInvURLPath(parsedTestResultName.invocationId)}
              >
                {parsedTestResultName.invocationId}
              </Link>
              :
            </div>
            <ul>
              {invArtifactsQuery.data.artifacts.map((artifact) => (
                <li key={artifact.name}>
                  <ArtifactLink artifact={artifact} />
                </li>
              ))}
            </ul>
          </>
        ) : (
          <></>
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
