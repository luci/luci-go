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

import Link from '@mui/material/Link';
import { useQuery } from '@tanstack/react-query';

import { useAuthState, useGetAccessToken } from '../../components/auth_state_provider';
import { PrpcClientExt } from '../../libs/prpc_client_ext';
import { getCodeSourceUrl } from '../../libs/url_utils';
import { QueryTestMetadataRequest, ResultDb } from '../../services/resultdb';

export interface TestIdLabelProps {
  readonly realm: string;
  readonly testId: string;
}

const MAIN_GIT_REF = 'refs/heads/main';

// TODO: query with pagination.
function useTestMetadata(req: QueryTestMetadataRequest) {
  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [identity, ResultDb.SERVICE, 'QueryTestMetadata', req],
    queryFn: async () => {
      const resultDBService = new ResultDb(new PrpcClientExt({ host: CONFIGS.RESULT_DB.HOST }, getAccessToken));
      const res = await resultDBService.queryTestMetadata(req, { acceptCache: false, skipUpdate: true });
      if (!res.testMetadata) {
        return {};
      }
      // Select the main branch. Fallback to the first element if main branch not found.
      const selected = res.testMetadata.find((m) => m.sourceRef.gitiles?.ref === MAIN_GIT_REF) || res.testMetadata[0];
      const testLocation = selected.testMetadata?.location;
      return {
        metadata: selected.testMetadata,
        sourceURL: testLocation ? getCodeSourceUrl(testLocation, selected.sourceRef.gitiles?.ref) : null,
      };
    },
  });
}

export function TestIdLabel({ realm, testId }: TestIdLabelProps) {
  const { data, isSuccess, isLoading } = useTestMetadata({ project: realm, predicate: { testIds: [testId] } });
  return (
    <table
      css={{
        width: '100%',
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        fontFamily: "'Google Sans', 'Helvetica Neue', sans-serif",
        fontSize: '14px',
      }}
    >
      <tbody>
        <tr>
          <td
            css={{
              color: 'var(--light-text-color)',
              /* Shrink the first column */
              width: '0px',
            }}
          >
            Project
          </td>
          <td>{realm}</td>
        </tr>
        <tr>
          <td css={{ color: 'var(--light-text-color)' }}>ID</td>
          <td>{testId}</td>
        </tr>
        {!isLoading && isSuccess && (
          <>
            {data.metadata?.name && (
              <tr>
                <td css={{ color: 'var(--light-text-color)' }}>Test</td>
                {data.sourceURL ? (
                  <td>
                    <Link href={data.sourceURL} target="_blank">
                      {data.metadata.name}
                    </Link>
                  </td>
                ) : (
                  <td>{data.metadata.name}</td>
                )}
              </tr>
            )}
          </>
        )}
      </tbody>
    </table>
  );
}
