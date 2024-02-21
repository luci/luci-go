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
import { Link, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useIssueListQuery } from '@/common/hooks/gapi_query/corp_issuetracker';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { Alerts } from '@/monitoring/components/alerts';
import { configuredTrees } from '@/monitoring/util/config';
import {
  AlertJson,
  AnnotationJson,
  bugFromJson,
} from '@/monitoring/util/server_json';
import {
  AlertsClientImpl,
  ListAlertsRequest,
} from '@/proto/infra/appengine/sheriff-o-matic/proto/v1/alerts.pb';

export const MonitoringPage = () => {
  const { tree: treeName } = useParams();
  const tree = configuredTrees.filter((t) => t.name === treeName)?.[0];

  const bugQuery = useIssueListQuery(
    {
      query: `hotlistid:${tree.hotlistId} status:open`,
      orderBy: 'priority',
    },
    {
      enabled: !!tree?.hotlistId,
    },
  );

  const client = usePrpcServiceClient({
    host: SETTINGS.sheriffOMatic.host,
    ClientImpl: AlertsClientImpl,
  });
  const alerts = useQuery({
    ...client.ListAlerts.query(
      ListAlertsRequest.fromPartial({
        parent: `trees/${treeName}`,
      }),
    ),
    refetchInterval: 60000,
    enabled: !!(treeName && tree),
  });
  // TODO(b/319315200): This doesn't work - replace with RPC when available.
  const { data: annotations } = useQuery({
    queryKey: ['annotations', treeName],
    queryFn: async () => {
      const response = await fetch(`api/v1/annotations/${treeName}`);
      if (!response.ok) {
        throw new Error(response.statusText);
      }
      const json: AnnotationJson[] = await response.json();
      const annotations: { [key: string]: AnnotationJson } = {};
      json.forEach((annotation) => {
        annotations[annotation.key] = annotation;
      });
      return annotations;
    },
    refetchInterval: 60000,
    enabled: !!(treeName && tree),
  });

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

  if (alerts.isLoading) {
    return <CircularProgress />;
  }
  if (alerts.isError) {
    throw alerts.error;
  }
  return (
    <>
      {bugQuery.isLoading ? <LinearProgress /> : null}
      {bugQuery.isError ? (
        <Alert severity="error">
          Failed to fetch bugs: {(bugQuery.error as Error).message}
        </Alert>
      ) : null}
      <Alerts
        alerts={alerts.data.alerts.map(
          (a) => JSON.parse(a.alertJson) as AlertJson,
        )}
        tree={tree}
        bugs={bugs}
        annotations={annotations || {}}
      />
    </>
  );
};

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="monitoring-page">
    <MonitoringPage />
  </RecoverableErrorBoundary>
);
