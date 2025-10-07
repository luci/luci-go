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

import { uniq } from 'lodash-es';
import { useEffect, useMemo } from 'react';

import { useInfiniteIssueListQuery } from '@/common/hooks/gapi_query/corp_issuetracker';
import { Bug, bugFromJson } from '@/monitoringv2/util/server_json';

import { useTree } from '../pages/monitoring_page/context';

import { useAlertGroups } from './alert_groups';

export interface BugsData {
  otherHotlistBugs: Bug[];
  groupBugs: { [groupName: string]: Bug[] }; // Bugs associated with alert groups.
  bugQueryError: unknown;
  isBugQueryError: boolean;
  bugIsRefetchError: boolean;
  bugIsLoading: boolean;
}

export function useBugs(): BugsData {
  const tree = useTree();
  const { alertGroups } = useAlertGroups();

  const linkedBugs = uniq(alertGroups.flatMap((g) => g.bugs));

  const {
    hasNextPage: bugHasNextPage,
    fetchNextPage: bugFetchNextPage,
    data: bugData,
    error: bugQueryError,
    isError: isBugQueryError,
    isRefetchError: bugIsRefetchError,
    isLoading: bugIsLoading,
  } = useInfiniteIssueListQuery(
    {
      query: `(status:open AND hotlistid:${tree?.hotlistId})${
        linkedBugs.length > 0 ? ' OR ' : ''
      }${linkedBugs.map((b) => 'id:' + b).join(' OR ')}`,
      orderBy: 'priority',
      pageSize: 50,
    },
    {
      refetchInterval: 60000,
      enabled: !!tree?.hotlistId,
    },
  );

  useEffect(() => {
    if (bugHasNextPage) {
      bugFetchNextPage();
    }
  }, [bugFetchNextPage, bugHasNextPage, bugData?.pages.length]);

  const bugs = useMemo(
    () =>
      bugData?.pages.flatMap((p) => p.issues || []).map((i) => bugFromJson(i)),
    [bugData],
  );

  const hotlistBugs = useMemo(
    () =>
      bugs?.filter((b) =>
        alertGroups.every((g) => !g.bugs.includes(b.number)),
      ) || [],
    [bugs, alertGroups],
  );

  const groupBugs = useMemo(() => {
    const result: { [groupName: string]: Bug[] } = {};
    for (const group of alertGroups) {
      result[group.name] =
        bugs?.filter((b) => group.bugs.includes(b.number)) || [];
    }
    return result;
  }, [alertGroups, bugs]);

  return {
    otherHotlistBugs: hotlistBugs,
    groupBugs,
    bugQueryError,
    isBugQueryError,
    bugIsRefetchError,
    bugIsLoading,
  };
}
