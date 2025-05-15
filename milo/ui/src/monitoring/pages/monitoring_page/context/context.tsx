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

import { useQuery, useQueries } from '@tanstack/react-query';
import { uniq, chunk } from 'lodash-es';
import { createContext, ReactNode, useEffect, useMemo } from 'react';

import { useInfiniteIssueListQuery } from '@/common/hooks/gapi_query/corp_issuetracker';
import {
  useNotifyAlertsClient,
  useSoMAlertsClient,
} from '@/monitoring/hooks/prpc_clients';
import {
  AlertExtensionJson,
  AlertJson,
  Bug,
  bugFromJson,
  TreeJson,
} from '@/monitoring/util/server_json';
import { ListAlertsRequest } from '@/proto/go.chromium.org/infra/appengine/sheriff-o-matic/proto/v1/alerts.pb';
import { BatchGetAlertsRequest } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

export interface ExtendedAlert {
  readonly bug: string;
  readonly silenceUntil: string | undefined;
  readonly key: string;
  readonly title: string;
  readonly body: string;
  readonly severity: number;
  readonly time: number;
  readonly start_time: number;
  readonly links: null;
  readonly tags: null;
  readonly type: string;
  readonly extension: AlertExtensionJson;
  readonly resolved: boolean;
}

interface MonitoringContext {
  readonly tree?: TreeJson;
  readonly alerts?: ExtendedAlert[];
  readonly alertsLoading?: boolean;
  readonly bugs?: Bug[];
  readonly bugsError?: Error | unknown;
  readonly isBugsError?: boolean;
  readonly bugsLoading?: boolean;
}

interface BugsQueryError extends Error {
  readonly result: {
    error: {
      message: string;
    };
  };
}

export const MonitoringCtx = createContext<MonitoringContext | null>(null);

interface Props {
  children: ReactNode;
  treeName: string | undefined;
  tree?: TreeJson;
}

export function MonitoringProvider({ children, treeName, tree }: Props) {
  const client = useSoMAlertsClient();
  const alertsQuery = useQuery({
    ...client.ListAlerts.query(
      ListAlertsRequest.fromPartial({
        parent: `trees/${treeName}`,
      }),
    ),
    refetchInterval: 60000,
    // Do not keep previous data otherwise we might be rendering alerts from a
    // different tree when user change the selected tree.
    enabled: !!(treeName && tree),
  });

  const notifyClient = useNotifyAlertsClient();
  // Eventually all of the data will come from LUCI Notify, but for now we just extend the
  // SOM alerts with the LUCI Notify alerts.
  const batches = chunk(alertsQuery.data?.alerts || [], 100);
  const extendedAlertsQuery = useQueries({
    queries: batches.map((batch) => ({
      ...notifyClient.BatchGetAlerts.query(
        BatchGetAlertsRequest.fromPartial({
          names: batch.map((a) => `alerts/${encodeURIComponent(a.key)}`),
        }),
      ),
      refetchInterval: 60000,
      keepPreviousData: true,
      enabled: !!(treeName && tree && alertsQuery.data),
    })),
  });

  const extendedAlertsData = Object.fromEntries(
    extendedAlertsQuery
      .flatMap((result) => result?.data?.alerts || [])
      .map((a) => [a.name, a]),
  );

  const linkedBugs = uniq(
    Object.values(extendedAlertsData)
      .map((a) => a?.bug)
      .filter((b) => b && b !== '0'),
  );

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
      enabled:
        !!tree?.hotlistId && !extendedAlertsQuery.some((q) => q.isLoading),
    },
  );

  useEffect(() => {
    if (bugHasNextPage) {
      bugFetchNextPage();
    }
  }, [bugFetchNextPage, bugHasNextPage]);

  if (alertsQuery.isError) {
    throw alertsQuery.error;
  }
  if (extendedAlertsQuery.some((q) => q.isError)) {
    throw extendedAlertsQuery.find((q) => q.isError && q.error);
  }

  const bugs = useMemo(
    () =>
      bugData?.pages.flatMap((p) => p.issues).map((i) => bugFromJson(i)) || [],
    [bugData],
  );

  const alerts = alertsQuery.data?.alerts.map((a) => {
    const extended = extendedAlertsData[`alerts/${encodeURIComponent(a.key)}`];
    const bug = extended?.bug;
    return {
      ...(JSON.parse(a.alertJson) as AlertJson),
      bug: !bug || bug === '0' ? '' : bug,
      silenceUntil: extended?.silenceUntil,
    };
  });
  const bugsQueryError = bugQueryError as BugsQueryError;
  return (
    <MonitoringCtx.Provider
      value={{
        tree,
        alerts,
        alertsLoading:
          alertsQuery.isLoading || extendedAlertsQuery.some((q) => q.isLoading),
        bugs,
        bugsError:
          bugsQueryError !== null
            ? new Error(bugsQueryError.result.error.message)
            : null,
        isBugsError: isBugQueryError || bugIsRefetchError,
        bugsLoading:
          (linkedBugs.length > 0 || !!tree?.hotlistId) && bugIsLoading,
      }}
    >
      {children}
    </MonitoringCtx.Provider>
  );
}
