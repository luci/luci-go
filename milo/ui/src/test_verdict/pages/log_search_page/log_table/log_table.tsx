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

import { GrpcError, ProtocolError } from '@chopsui/prpc-client';
import { Box, Alert, AlertTitle } from '@mui/material';
import LinearProgress from '@mui/material/LinearProgress';
import { useQuery } from '@tanstack/react-query';

import {
  ParamsPager,
  getPageSize,
  getPageToken,
} from '@/common/components/params_pager';
import { PagerContext } from '@/common/components/params_pager/context';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { QueryTestVariantArtifactGroupsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { OutputQueryTestVariantArtifactGroupsResponse } from '@/test_verdict/types';

import { FormData, CompleteFormToSearch } from '../form_data';

import { LogGroup } from './log_group';

export interface LogSearchProps {
  readonly project: string;
  readonly pagerCtx: PagerContext;
  readonly form: CompleteFormToSearch;
}

// TODO (beining@):
// * search for invocation artifact, and display on a different tab.
// * link to log viewer.
export function LogTable({ project, form, pagerCtx }: LogSearchProps) {
  const [searchParams] = useSyncedSearchParams();
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);
  const client = useResultDbClient();
  const searchString = FormData.getSearchString(form);
  const { data, isLoading, error, isError } = useQuery({
    ...client.QueryTestVariantArtifactGroups.query(
      QueryTestVariantArtifactGroupsRequest.fromPartial({
        project: project,
        searchString,
        testIdMatcher: FormData.getTestIDMatcher(form),
        artifactIdMatcher: FormData.getArtifactIDMatcher(form),
        startTime: form.startTime ? form.startTime.toISO() : '',
        endTime: form.endTime ? form.endTime.toISO() : '',
        pageSize: pageSize,
        pageToken: pageToken,
      }),
    ),
    select: (data) => data as OutputQueryTestVariantArtifactGroupsResponse,
  });

  if (isError) {
    const isReqError =
      error instanceof GrpcError || error instanceof ProtocolError;
    if (isReqError) {
      return (
        <Alert severity="error">
          <AlertTitle>Failed to load matching logs</AlertTitle>
          {error.message}
        </Alert>
      );
    }
    throw error;
  }

  return (
    <>
      <Box
        sx={{
          padding: '10px 0px',
        }}
      >
        {isLoading ? (
          <LinearProgress />
        ) : (
          <>
            {data.groups.length === 0 && (
              <Box sx={{ padding: '0px 15px' }}>no matching artifact</Box>
            )}
            {data.groups.map((g) => (
              <LogGroup
                project={project}
                group={g}
                key={g.testId + g.variantHash + g.artifactId}
              />
            ))}
          </>
        )}
        <ParamsPager
          pagerCtx={pagerCtx}
          nextPageToken={data?.nextPageToken || ''}
        />
      </Box>
    </>
  );
}
