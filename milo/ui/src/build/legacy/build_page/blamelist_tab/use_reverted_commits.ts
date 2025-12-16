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

import { useQuery } from '@tanstack/react-query';

import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { ProxyGitilesLogRequest } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

// A map from a reverted commit ID to the reverting commit.
export type RevertedCommitsMap = ReadonlyMap<string, GitilesCommit>;

export function useRevertedCommits(blamelistPin: GitilesCommit): {
  revertedCommitsMap?: RevertedCommitsMap;
  isLoadingReverts: boolean;
} {
  const sevenDaysAgo = new Date();
  sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);

  const client = useMiloInternalClient();
  const { data: blamelistPinCommit, isInitialLoading: isLoadingBlamelistPin } =
    useQuery({
      queryKey: [
        'commit',
        blamelistPin.host,
        blamelistPin.project,
        blamelistPin.id,
      ],
      queryFn: async () => {
        const res = await client.ProxyGitilesLog(
          ProxyGitilesLogRequest.fromPartial({
            host: blamelistPin.host,
            request: {
              project: blamelistPin.project,
              committish: blamelistPin.id,
              pageSize: 1,
            },
          }),
        );
        return res.log?.[0];
      },
      // This query is cheap and the result is stable.
      refetchOnWindowFocus: false,
      staleTime: Infinity,
    });

  const blamelistPinTime = blamelistPinCommit?.committer?.time
    ? new Date(blamelistPinCommit.committer.time)
    : undefined;

  // TODO(jiameil): use gerrit search once an RPC method is available.
  const {
    data: revertedCommitsMap,
    isInitialLoading: isLoadingRevertedCommits,
  } = useQuery({
    queryKey: ['revertedCommits', blamelistPin.host, blamelistPin.project],
    enabled: !!blamelistPinTime && blamelistPinTime >= sevenDaysAgo,
    queryFn: async () => {
      const revertedCommitsMap = new Map<string, GitilesCommit>();
      let pageToken = '';

      while (true) {
        const res = await client.ProxyGitilesLog(
          ProxyGitilesLogRequest.fromPartial({
            host: blamelistPin.host,
            request: {
              project: blamelistPin.project,
              committish: 'HEAD',
              pageSize: 1000,
              pageToken,
            },
          }),
        );

        if (!res.log) {
          break;
        }

        let oldestCommitInPage: Date | undefined;
        for (const commit of res.log) {
          if (commit.committer?.time) {
            const commitTime = new Date(commit.committer.time);
            if (!oldestCommitInPage || commitTime < oldestCommitInPage) {
              oldestCommitInPage = commitTime;
            }
          }

          if (!commit.message.startsWith('Revert ')) {
            continue;
          }

          const match = /This reverts commit ([a-f0-9]{7,40})\./.exec(
            commit.message,
          );
          if (match) {
            const revertedCommitId = match[1];
            revertedCommitsMap.set(revertedCommitId, {
              host: blamelistPin.host,
              project: blamelistPin.project,
              id: commit.id,
              ref: '',
              position: 0,
            });
          }
        }

        if (res.nextPageToken) {
          pageToken = res.nextPageToken;
        } else {
          break;
        }

        if (oldestCommitInPage && oldestCommitInPage < sevenDaysAgo) {
          break;
        }
      }
      return revertedCommitsMap;
    },
    // The query is expensive and the reverted commits should be stable anyway.
    refetchOnWindowFocus: false,
    staleTime: Infinity,
  });

  return {
    revertedCommitsMap,
    isLoadingReverts: isLoadingBlamelistPin || isLoadingRevertedCommits,
  };
}
