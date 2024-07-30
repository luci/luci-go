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

import {
  InsertDriveFile,
  UnfoldMore,
  ArrowForwardIos,
} from '@mui/icons-material';
import { Box, Link, styled } from '@mui/material';
import LinearProgress from '@mui/material/LinearProgress';
import { InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { getTestHistoryURLPath } from '@/common/tools/url_utils';
import { Variant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import {
  QueryTestVariantArtifactGroupsRequest,
  ArtifactContentMatcher,
  IDMatcher,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import {
  OutputQueryTestVariantArtifactGroupsResponse_MatchGroup,
  OutputQueryTestVariantArtifactGroupsResponse,
} from '@/test_verdict/types';

import { FormData } from '../form_data';

import { LogSnippetRow } from './log_snippet_row';

const LogGroupHeaderDiv = styled(Box)`
  background: #e8f0fe;
  padding: 2px 5px;
  display: flex;
  align-items: center;
  font-size: 17px;
  line-height: 1.5;
  letter-spacing: normal;
  gap: 5px;
  flex-wrap: wrap;
`;

const ExpandableRowDiv = styled(Box)`
  text-align: center;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 10px 0px 0px 0px;
`;

export interface LogSearchProps {
  readonly project: string;
  readonly form: FormData;
}

function getSearchString(form: FormData): ArtifactContentMatcher | undefined {
  return form.searchStr === ''
    ? undefined
    : form.isSearchStrRegex
      ? { regexContain: form.searchStr }
      : { exactContain: form.searchStr };
}

function getTestIDMatcher(form: FormData): IDMatcher | undefined {
  return form.testIDStr === ''
    ? undefined
    : form.isTestIDStrPrefix
      ? { hasPrefix: form.testIDStr }
      : { exactEqual: form.testIDStr };
}

function getArtifactIDMatcher(form: FormData): IDMatcher | undefined {
  return form.artifactIDStr === ''
    ? undefined
    : form.isArtifactIDStrPrefix
      ? { hasPrefix: form.artifactIDStr }
      : { exactEqual: form.artifactIDStr };
}

// TODO (beining@):
// * Pagination
// * Expand test variant artifact groups.
// * search for invocation artifact, and display on a different tab.
// * link to log viewer.
// * handle RPC error.
export function LogTable({ project, form }: LogSearchProps) {
  const client = useResultDbClient();

  const { data, isLoading, error, isError } = useInfiniteQuery({
    ...client.QueryTestVariantArtifactGroups.queryPaged(
      QueryTestVariantArtifactGroupsRequest.fromPartial({
        project: project,
        searchString: getSearchString(form),
        testIdMatcher: getTestIDMatcher(form),
        artifactIdMatcher: getArtifactIDMatcher(form),
        startTime: form.startTime ? form.startTime.toString() : '',
        endTime: form.endTime ? form.endTime.toString() : '',
      }),
    ),
    select: (data) =>
      data as InfiniteData<OutputQueryTestVariantArtifactGroupsResponse>,
  });

  if (isError) {
    throw error;
  }

  const groups = useMemo(
    () => data?.pages.flatMap((p) => p.groups) || [],
    [data],
  );
  if (isLoading) {
    return <LinearProgress />;
  }
  return (
    <Box
      sx={{
        padding: '10px 0px',
      }}
    >
      {groups.length === 0 && (
        <Box sx={{ padding: '0px 15px' }}>no matching artifact</Box>
      )}
      {groups.map((g) => (
        <LogGroup
          project={project}
          group={g}
          key={g.testId + g.variantHash + g.artifactId}
        />
      ))}
    </Box>
  );
}

function variantToString(v: Variant) {
  return Object.entries(v.def)
    .map(([k, v]) => `${k}:${v}`)
    .join(', ');
}

interface LogGroupProps {
  readonly project: string;
  readonly group: OutputQueryTestVariantArtifactGroupsResponse_MatchGroup;
}

function LogGroup({ project, group }: LogGroupProps) {
  const { testId, variant, artifactId, artifacts, matchingCount } = group;
  return (
    <>
      <LogGroupHeaderDiv>
        <InsertDriveFile color="action" sx={{ fontSize: '17px' }} />
        <Link
          href={getTestHistoryURLPath(project, testId)}
          color="inherit"
          underline="hover"
          target="_blank"
          rel="noopenner"
        >
          {testId}
        </Link>
        <ArrowForwardIos sx={{ fontSize: '17px' }} />
        <Box>{variant && variantToString(variant)} </Box>
        <ArrowForwardIos sx={{ fontSize: '17px' }} />
        {artifactId}
      </LogGroupHeaderDiv>
      {artifacts.map((a) => (
        <LogSnippetRow artifact={a} key={a.name} />
      ))}

      {matchingCount - artifacts.length > 0 && (
        <ExpandableRowDiv>
          <UnfoldMore sx={{ fontSize: '20px' }} /> Show{' '}
          {matchingCount - artifacts.length} more matching log for this test
          varaint artifact
        </ExpandableRowDiv>
      )}
    </>
  );
}
