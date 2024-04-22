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

import { Alert, CircularProgress, LinearProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { uniq } from 'lodash-es';
import { Link, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useIssueListQuery } from '@/common/hooks/gapi_query/corp_issuetracker';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { Alerts } from '@/monitoring/components/alerts';
import { configuredTrees } from '@/monitoring/util/config';
import { AlertJson, bugFromJson } from '@/monitoring/util/server_json';
import {
  BatchGetAlertsRequest,
  AlertsClientImpl as NotifyAlertsClientImpl,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';
import {
  AlertsClientImpl,
  ListAlertsRequest,
} from '@/proto/infra/appengine/sheriff-o-matic/proto/v1/alerts.pb';

export const MonitoringPage = () => {
  const { tree: treeName } = useParams();
  const tree = configuredTrees.filter((t) => t.name === treeName)?.[0];

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
  // Eventually all of the deata will come from LUCI Notify, but for now we just extend the
  // SOM alerts with the LUCI Notify alerts.
  const extendedQuery = useQuery({
    ...notifyClient.BatchGetAlerts.query(
      BatchGetAlertsRequest.fromPartial({
        names: alertsQuery.data?.alerts?.map(
          (a) => `alerts/${encodeURIComponent(a.key)}`,
        ),
      }),
    ),
    refetchInterval: 60000,
    enabled: !!(treeName && tree && alertsQuery.data),
  });

  const linkedBugs = uniq(
    (extendedQuery.data?.alerts || [])
      .map((a) => a.bug)
      .filter((b) => b !== '0'),
  );

  const bugQuery = useIssueListQuery(
    {
      query: `(status:open AND hotlistid:${tree.hotlistId})${
        linkedBugs.length > 0 ? ' OR ' : ''
      }${linkedBugs.map((b) => 'id:' + b).join(' OR ')}`,
      orderBy: 'priority',
    },
    {
      refetchInterval: 60000,
      enabled: !!tree?.hotlistId && !!extendedQuery.data,
    },
  );

  if (!treeName || !tree) {
    return (
      <>
        <p>Please choose a tree to monitor:</p>
        <ul>
          {configuredTrees.map((t) => (
            <li key={t.name}>
              <Link to={`/ui/labs/monitoring/${t.name}`}>{t.name}</Link>
            </li>
          ))}
        </ul>
      </>
    );
  }
  const bugs = bugQuery.data?.issues?.map((i) => bugFromJson(i)) || [];

  if (alertsQuery.isError) {
    throw alertsQuery.error;
  }
  if (extendedQuery.isError) {
    throw extendedQuery.error;
  }
  if (alertsQuery.isLoading || extendedQuery.isLoading) {
    return <CircularProgress />;
  }

  // Extend the alerts with the LUCI Notify data.
  const alerts = alertsQuery.data.alerts.map((a, i) => {
    const bug = extendedQuery.data.alerts[i].bug;
    return {
      ...(JSON.parse(a.alertJson) as AlertJson),
      bug: !bug || bug === '0' ? '' : bug,
      silenceUntil: extendedQuery.data.alerts[i].silenceUntil,
    };
  });
  return (
    <>
      {bugQuery.isLoading ? <LinearProgress /> : null}
      {bugQuery.isError ? (
        <Alert severity="error">
          Failed to fetch bugs: {(bugQuery.error as Error).message}
        </Alert>
      ) : null}
      <Alerts alerts={alerts} tree={tree} bugs={bugs} />
    </>
  );
};

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="monitoring-page">
    <MonitoringPage />
  </RecoverableErrorBoundary>
);
