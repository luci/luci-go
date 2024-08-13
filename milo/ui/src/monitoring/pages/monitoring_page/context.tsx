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
import { createContext, ReactNode } from 'react';

import { useIssueListQuery } from '@/common/hooks/gapi_query/corp_issuetracker';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import {
  AlertExtensionJson,
  AlertJson,
  Bug,
  bugFromJson,
  TreeJson,
} from '@/monitoring/util/server_json';
import {
  BatchGetAlertsRequest,
  AlertsClientImpl as NotifyAlertsClientImpl,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';
import {
  AlertsClientImpl,
  ListAlertsRequest,
} from '@/proto/infra/appengine/sheriff-o-matic/proto/v1/alerts.pb';

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

export const MonitoringCtx = createContext<MonitoringContext | null>(null);

interface Props {
  children: ReactNode;
  treeName: string | undefined;
  tree: TreeJson;
}

export function MonitoringProvider({ children, treeName, tree }: Props) {
  const client = usePrpcServiceClient({
    host: SETTINGS.sheriffOMatic.host,
    ClientImpl: AlertsClientImpl,
  });
  const alertsQuery = useQuery({
    ...client.ListAlerts.query(
      ListAlertsRequest.fromPartial({
        parent: `trees/${treeName}`,
      }),
    ),
    refetchInterval: 60000,
    enabled: !!(treeName && tree),
  });

  const notifyClient = usePrpcServiceClient({
    host: SETTINGS.luciNotify.host,
    ClientImpl: NotifyAlertsClientImpl,
  });
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
      enabled: !!(treeName && tree && alertsQuery.data),
    })),
  });

  const extendedAlertsData = extendedAlertsQuery.flatMap(
    (result) => result?.data?.alerts,
  );
  const linkedBugs = uniq(
    (extendedAlertsData || []).map((a) => a?.bug).filter((b) => b && b !== '0'),
  );

  const bugQuery = useIssueListQuery(
    {
      query: `(status:open AND hotlistid:${tree?.hotlistId})${
        linkedBugs.length > 0 ? ' OR ' : ''
      }${linkedBugs.map((b) => 'id:' + b).join(' OR ')}`,
      orderBy: 'priority',
    },
    {
      refetchInterval: 60000,
      enabled:
        !!tree?.hotlistId && !extendedAlertsQuery.some((q) => q.isLoading),
    },
  );

  if (alertsQuery.isError) {
    throw alertsQuery.error;
  }
  if (extendedAlertsQuery.some((q) => q.isError)) {
    throw extendedAlertsQuery.find((q) => q.isError && q.error);
  }

  const bugs = bugQuery.data?.issues?.map((i) => bugFromJson(i));

  const alerts = alertsQuery.data?.alerts.map((a, i) => {
    const extended =
      extendedAlertsQuery[Math.floor(i / 100)].data?.alerts[i % 100];
    const bug = extended?.bug;
    return {
      ...(JSON.parse(a.alertJson) as AlertJson),
      bug: !bug || bug === '0' ? '' : bug,
      silenceUntil: extended?.silenceUntil,
    };
  });
  return (
    <MonitoringCtx.Provider
      value={{
        tree,
        alerts,
        alertsLoading:
          alertsQuery.isLoading ||
          alertsQuery.isRefetching ||
          extendedAlertsQuery.some((q) => q.isLoading || q.isRefetching),
        bugs,
        bugsError: bugQuery.error,
        isBugsError: bugQuery.isError || bugQuery.isRefetchError,
        bugsLoading: bugQuery.isLoading || bugQuery.isRefetching,
      }}
    >
      {children}
    </MonitoringCtx.Provider>
  );
}
