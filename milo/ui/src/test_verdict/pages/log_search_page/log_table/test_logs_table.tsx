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
import { ArrowForwardIos } from '@mui/icons-material';
import { Box, Alert, AlertTitle, Link, styled } from '@mui/material';
import LinearProgress from '@mui/material/LinearProgress';
import { useQuery } from '@tanstack/react-query';

import {
  ParamsPager,
  getPageSize,
  getPageToken,
} from '@/common/components/params_pager';
import {
  getTestHistoryURLWithSearchParam,
  generateTestHistoryURLSearchParams,
} from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { QueryTestVariantArtifactGroupsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { OutputQueryTestVariantArtifactGroupsResponse } from '@/test_verdict/types';

import { SearchFilter, useTestLogPagerCtx } from '../context';
import { FormData } from '../form_data';
import { VariantLine } from '../variant_line';

import { LogGroup } from './log_group';

export const SlowQueryTextBox = styled(Box)`
  background: #e0f0ff;
  margin: 10px;
  padding: 20px;
  border-radius: 10px;
  font-size: 17px;
`;

export interface TestLogsTableProps {
  readonly project: string;
  readonly filter: SearchFilter;
}

export function TestLogsTable({ project, filter }: TestLogsTableProps) {
  const [searchParams] = useSyncedSearchParams();
  const pagerCtx = useTestLogPagerCtx();
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);
  const client = useResultDbClient();
  const { form, startTime, endTime } = filter;
  const { data, isLoading, error, isError } = useQuery({
    ...client.QueryTestVariantArtifactGroups.query(
      QueryTestVariantArtifactGroupsRequest.fromPartial({
        project: project,
        searchString: FormData.getSearchString(form),
        testIdMatcher: FormData.getTestIDMatcher(form),
        artifactIdMatcher: FormData.getArtifactIDMatcher(form),
        startTime: startTime.toISO(),
        endTime: endTime.toISO(),
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
          <>
            <LinearProgress />
            {form.testIDStr === '' && (
              <SlowQueryTextBox>
                Searching for test result logs... <br /> If the query is slow,
                we highly recommend you to scope down the search by providing a
                <strong> test id or test id prefix.</strong> This can make the
                query 10-100x faster!
              </SlowQueryTextBox>
            )}
          </>
        ) : (
          <>
            {data.groups.length === 0 && (
              <Box sx={{ padding: '0px 15px' }}>no matching logs</Box>
            )}
            {data.groups.map((g) => (
              <LogGroup
                key={g.testId + g.variantHash + g.artifactId}
                dialogAction={{
                  type: 'showTestLogGroupList',
                  logGroupIdentifer: {
                    testID: g.testId,
                    variantHash: g.variantHash,
                    variant: g.variant,
                    artifactID: g.artifactId,
                  },
                }}
                group={g}
                groupHeader={
                  <>
                    <Link
                      href={getTestHistoryURLWithSearchParam(
                        project,
                        g.testId,
                        generateTestHistoryURLSearchParams(
                          g.variant || { def: {} },
                        ),
                      )}
                      color="inherit"
                      underline="hover"
                      target="_blank"
                      rel="noopenner"
                    >
                      {g.testId}
                    </Link>
                    <ArrowForwardIos sx={{ fontSize: 'inherit' }} />
                    <Box>
                      {g.variant && <VariantLine variant={g.variant} />}
                    </Box>
                    <ArrowForwardIos sx={{ fontSize: 'inherit' }} />
                    {g.artifactId}
                  </>
                }
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
