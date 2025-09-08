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
import { chunk, uniq, uniqBy, uniqWith } from 'lodash-es';
import { createContext, ReactNode } from 'react';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  useNotifyAlertsClient,
  useSoMAlertsClient,
} from '@/monitoringv2/hooks/prpc_clients';
import {
  BuilderAlert,
  OneBuildHistory,
  OneTestHistory,
  StepAlert,
  TestAlert,
} from '@/monitoringv2/util/alerts';
import {
  AlertExtensionJson,
  AlertJson,
  builderPath,
  TreeJson,
} from '@/monitoringv2/util/server_json';
import { ListAlertsRequest } from '@/proto/go.chromium.org/infra/appengine/sheriff-o-matic/proto/v1/alerts.pb';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { SearchBuildsRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { BatchGetAlertsRequest } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';
import { QueryTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

const FIELD_MASK = Object.freeze([
  'builds.*.builder',
  'builds.*.id',
  'builds.*.status',
  'builds.*.startTime',
  'builds.*.summaryMarkdown',
  'builds.*.steps.*.name',
  'builds.*.steps.*.status',
  'builds.*.steps.*.summaryMarkdown',
]);

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
  readonly alertsLoadingStatus?: string;
  readonly builderAlerts: BuilderAlert[];
  readonly stepAlerts: StepAlert[];
  readonly testAlerts: TestAlert[];
}

interface BuildAndTestVariants {
  build: Build;
  testVariants: TestVariant[];
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

  const alertJsons = (alertsQuery.data?.alerts || []).map(
    (a) => JSON.parse(a.alertJson) as AlertJson,
  );
  const uniqueBuilders = uniqBy(
    alertJsons.flatMap((a) =>
      a.extension.builders.map((b) => ({
        project: b.project,
        bucket: b.bucket,
        builder: b.name,
      })),
    ),
    builderPath,
  );

  const bbClient = useBuildsClient();
  const historiesQueries = useQueries({
    queries: uniqueBuilders.map((builderId) => {
      const req = SearchBuildsRequest.fromPartial({
        predicate: {
          builder: builderId,
          includeExperimental: true,
          status: Status.ENDED_MASK,
        },
        pageSize: 10,
        fields: FIELD_MASK,
      });
      return {
        ...bbClient.SearchBuilds.query(req),
        staleTime: 60000,
        enabled: !!(treeName && tree && alertsQuery.data),
      };
    }),
  });

  const histories = historiesQueries
    .map(
      (q) =>
        q.data?.builds?.map((build) => {
          const btv: BuildAndTestVariants = { build, testVariants: [] };
          return btv;
        }) || [],
    )
    .filter((btvs) => !!btvs?.[0]?.build);
  const buildHistories = histories.flatMap((btvs) => btvs);

  const rdbClient = useResultDbClient();
  const failingTestsQueries = useQueries({
    queries: buildHistories.map((btv) => {
      const req = QueryTestVariantsRequest.fromPartial({
        invocations: [`invocations/build-${btv.build.id}`],
        pageSize: 50,
        resultLimit: 10,
        predicate: { status: TestVariantStatus.UNEXPECTED },
      });
      return {
        ...rdbClient.QueryTestVariants.query(req),
        staleTime: Infinity, // This is immutable data, no need to ever refetch.
        enabled: !!(treeName && tree && uniqueBuilders.length > 0),
      };
    }),
  });

  buildHistories.forEach(
    (btv, i) =>
      (btv.testVariants =
        failingTestsQueries[i].data?.testVariants?.slice() || []),
  );
  if (alertsQuery.isError) {
    throw alertsQuery.error;
  }
  if (extendedAlertsQuery.some((q) => q.isError)) {
    throw extendedAlertsQuery.find((q) => q.isError && q.error);
  }
  if (historiesQueries.some((q) => q.isError)) {
    throw historiesQueries.find((q) => q.isError && q.error);
  }
  if (failingTestsQueries.some((q) => q.isError)) {
    throw failingTestsQueries.find((q) => q.isError && q.error);
  }

  const builderAlerts = createBuilderAlerts(histories);
  const stepAlerts = createStepAlerts(histories);
  const testAlerts = createTestAlerts(histories);

  const alerts = alertsQuery.data?.alerts.map((a) => {
    const extended = extendedAlertsData[`alerts/${encodeURIComponent(a.key)}`];
    const bug = extended?.bug;
    return {
      ...(JSON.parse(a.alertJson) as AlertJson),
      bug: !bug || bug === '0' ? '' : bug,
      silenceUntil: extended?.silenceUntil,
    };
  });
  const alertsLoadingStatus = alertsQuery.isLoading
    ? 'Loading failing builders...'
    : extendedAlertsQuery.find((q) => q.isLoading)
      ? `Loading builder history (${extendedAlertsQuery.filter((q) => q.isLoading).length}/${extendedAlertsQuery.length})...`
      : failingTestsQueries.find((q) => q.isLoading)
        ? `Loading failing tests (${failingTestsQueries.filter((q) => q.isLoading).length}/${failingTestsQueries.length})...`
        : undefined;

  return (
    <MonitoringCtx.Provider
      value={{
        tree,
        alerts,
        alertsLoading:
          alertsQuery.isLoading ||
          extendedAlertsQuery.some((q) => q.isLoading) ||
          historiesQueries.some((q) => q.isLoading),
        alertsLoadingStatus,
        builderAlerts: builderAlerts,
        stepAlerts: stepAlerts,
        testAlerts: testAlerts,
      }}
    >
      {children}
    </MonitoringCtx.Provider>
  );
}

const createBuilderAlerts = (histories: BuildAndTestVariants[][]) => {
  return histories.map((history) => {
    const firstPassIndex = history.findIndex(
      (btv) => !btv.build.status || btv.build.status === Status.SUCCESS,
    );
    const firstFailIndex = history.findIndex(
      (btv) => btv.build.status && btv.build.status !== Status.SUCCESS,
    );
    const alert: BuilderAlert = {
      kind: 'builder',
      key: builderPath(history[0].build.builder!),
      builderID: history[0].build.builder!,
      history: history.map((build) => ({
        buildId: build.build.id,
        status: build.build.status,
        startTime: build.build.startTime,
        summaryMarkdown: build.build.summaryMarkdown,
      })),
      consecutiveFailures: firstPassIndex === -1 ? 10 : firstPassIndex,
      consecutivePasses: firstFailIndex === -1 ? 10 : firstFailIndex,
    };
    return alert;
  });
};

const createStepAlerts = (histories: BuildAndTestVariants[][]) => {
  return histories.flatMap(createStepAlertsforBuilder);
};

const createStepAlertsforBuilder = (history: BuildAndTestVariants[]) => {
  const failingStepNames = uniq(
    history.flatMap((btv) =>
      btv.build.steps
        .filter((step) => step.status !== Status.SUCCESS)
        .map((step) => step.name.replace(' (retry shards)', '')),
    ),
  );
  return failingStepNames.map((stepName) => {
    const stepHistory = history.map((btv) => {
      // The steps are in order, but we only want the last retry.
      const lastRetryStep = btv.build.steps
        .filter((step) => step.name.replace(' (retry shards)', '') === stepName)
        .at(-1);
      const h: OneBuildHistory = {
        buildId: btv.build.id,
        status: lastRetryStep?.status,
        startTime: btv.build.startTime,
        summaryMarkdown: lastRetryStep?.summaryMarkdown,
      };
      return h;
    });
    const firstPassIndex = stepHistory.findIndex(
      (h) => !h.status || h.status === Status.SUCCESS,
    );
    const firstFailIndex = stepHistory.findIndex(
      (h) => h.status && h.status !== Status.SUCCESS,
    );
    const alert: StepAlert = {
      kind: 'step',
      key: `${builderPath(history[0].build.builder!)}/${stepName}`,
      builderID: history[0].build.builder!,
      stepName,
      history: stepHistory,
      consecutiveFailures: firstPassIndex === -1 ? 10 : firstPassIndex,
      consecutivePasses: firstFailIndex === -1 ? 10 : firstFailIndex,
    };
    return alert;
  });
};

const createTestAlerts = (histories: BuildAndTestVariants[][]) => {
  return histories.flatMap(createTestAlertsForBuilder);
};

const createTestAlertsForBuilder = (history: BuildAndTestVariants[]) => {
  const intermediate = history.flatMap((btv) =>
    btv.testVariants.filter((tv) => tv.status !== TestVariantStatus.EXPECTED),
  );
  const failingTests = uniqWith(
    intermediate,
    (a, b) => a.testId === b.testId && a.variantHash === b.variantHash,
  );
  return failingTests.map((failingTest) => {
    const testHistory = history.map((btv) => {
      const tv = btv.testVariants.find(
        (tv) =>
          tv.testId === failingTest.testId &&
          tv.variantHash === failingTest.variantHash,
      );
      const h: OneTestHistory = {
        buildId: btv.build.id,
        status: tv?.status,
        startTime: btv.build.startTime,
        failureReason: tv?.results.find(
          (r) => r.result?.failureReason?.primaryErrorMessage,
        )?.result?.failureReason?.primaryErrorMessage,
      };
      return h;
    });
    const firstPassIndex = testHistory.findIndex(
      (h) => !h.status || h.status === TestVariantStatus.EXPECTED,
    );
    const firstFailIndex = testHistory.findIndex(
      (h) => h.status && h.status !== TestVariantStatus.EXPECTED,
    );
    const stepName =
      failingTest.results[0].result?.tags
        .find((t) => t.key === 'step_name')
        ?.value?.replace(' (retry shards)', '') || 'Tests';
    const alert: TestAlert = {
      kind: 'test',
      key: `${builderPath(history[0].build.builder!)}/${stepName}/${failingTest.variantHash}/${failingTest.testId}`,
      builderID: history[0].build.builder!,
      stepName,
      testId: failingTest.testId,
      testName: failingTest.testMetadata?.name || failingTest.testId,
      variantHash: failingTest.variantHash,
      history: testHistory,
      consecutiveFailures: firstPassIndex === -1 ? 10 : firstPassIndex,
      consecutivePasses: firstFailIndex === -1 ? 10 : firstFailIndex,
    };
    return alert;
  });
};
