// Copyright 2023 The LUCI Authors.
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

import { Link, Typography } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { useTestHistoryClient } from '@/common/hooks/prpc_clients';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { QueryTestsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';

interface Props {
  searchQuery: string;
  project: string;
}

export function TestList({ project, searchQuery }: Props) {
  const client = useTestHistoryClient();
  const { data, isError, error, isPending, fetchNextPage, hasNextPage } =
    useInfiniteQuery({
      ...client.QueryTests.queryPaged(
        QueryTestsRequest.fromPartial({
          project: project,
          testIdSubstring: searchQuery,
          caseInsensitive: true,
        }),
      ),
      enabled: searchQuery !== '',
    });

  const testIds = useMemo(() => {
    const testIds = data?.pages.flatMap((p) => p.testIds) || [];
    return testIds;
  }, [data]);

  if (isError) {
    throw error;
  }

  return (
    <>
      <ul>
        {testIds.map((testId) => (
          <TestRow key={testId} project={project} testId={testId} />
        ))}
      </ul>
      {isPending && searchQuery !== '' ? (
        <Typography component="span">
          Loading
          <DotSpinner />
        </Typography>
      ) : (
        hasNextPage && (
          <Typography
            component="span"
            className="active-text"
            onClick={() => fetchNextPage()}
          >
            [load more]
          </Typography>
        )
      )}
      {testIds.length === 0 && searchQuery !== '' && !isPending && (
        <Typography component="span">
          No tests found with case insensitive substring search.
        </Typography>
      )}
    </>
  );
}

interface TestRowProps {
  project: string;
  testId: string;
}

const TestRow = ({ project, testId }: TestRowProps) => {
  return (
    <li key={testId}>
      <Link
        href={`/ui/test/${encodeURIComponent(
          // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
          project!,
        )}/${encodeURIComponent(testId)}`}
        target="_blank"
        rel="noreferrer"
      >
        {testId}
      </Link>
    </li>
  );
};
