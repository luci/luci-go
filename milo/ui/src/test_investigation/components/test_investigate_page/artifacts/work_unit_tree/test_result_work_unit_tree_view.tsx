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

import { CircularProgress } from '@mui/material';
import { useQueries, useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { WorkUnitPredicate } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  QueryWorkUnitsRequest,
  WorkUnit,
  GetWorkUnitRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';
import { WorkUnitArtifactTree } from '@/test_investigation/components/common/artifacts/tree/work_unit_artifact_tree';

import { useArtifactsContext } from '../context';

interface TestResultWorkUnitTreeViewProps {
  rootInvocationId: string;
  workUnitId: string;
}

export function TestResultWorkUnitTreeView({
  rootInvocationId,
  workUnitId,
}: TestResultWorkUnitTreeViewProps) {
  const resultDbClient = useResultDbClient();
  const { currentResult, setSelectedArtifact } = useArtifactsContext();
  const [searchParams] = useSyncedSearchParams();

  const { data: workUnitsData, isLoading: isLoadingWorkUnits } = useQuery(
    resultDbClient.QueryWorkUnits.query(
      QueryWorkUnitsRequest.fromPartial({
        parent: `rootInvocations/${rootInvocationId}`,
        predicate: WorkUnitPredicate.fromPartial({
          ancestorsOf: `rootInvocations/${rootInvocationId}/workUnits/${workUnitId}`,
        }),
      }),
    ),
  );

  const targetWorkUnitName = `rootInvocations/${rootInvocationId}/workUnits/${workUnitId}`;

  const { data: targetWorkUnitData, isLoading: isLoadingTargetWorkUnit } =
    useQuery(
      resultDbClient.GetWorkUnit.query(
        GetWorkUnitRequest.fromPartial({
          name: targetWorkUnitName,
        }),
      ),
    );

  const {
    data: testResultArtifactsData,
    isLoading: isLoadingTestResultArtifacts,
  } = useQuery(
    resultDbClient.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: currentResult?.name,
        pageSize: 1000,
      }),
    ),
  );

  const workUnitArtifactQueries = useQueries({
    queries: [
      ...(workUnitsData?.workUnits || []).map((wu) =>
        resultDbClient.ListArtifacts.query(
          ListArtifactsRequest.fromPartial({
            parent: wu.name,
            pageSize: 1000,
          }),
        ),
      ),
      resultDbClient.ListArtifacts.query(
        ListArtifactsRequest.fromPartial({
          parent: targetWorkUnitName,
          pageSize: 1000,
        }),
      ),
    ],
  });

  const artifactsByWorkUnit = useMemo(() => {
    const map: Record<string, readonly Artifact[]> = {};
    const workUnits = workUnitsData?.workUnits || [];

    // Process ancestors
    workUnits.forEach((wu, index) => {
      const query = workUnitArtifactQueries[index];
      if (wu && query.data) {
        map[wu.name] = query.data.artifacts || [];
      }
    });

    // Process target work unit (last query)
    const targetQuery = workUnitArtifactQueries[workUnits.length];
    if (targetQuery && targetQuery.data) {
      map[targetWorkUnitName] = targetQuery.data.artifacts || [];
    }

    return map;
  }, [workUnitArtifactQueries, workUnitsData, targetWorkUnitName]);

  const isLoadingWorkUnitArtifacts = workUnitArtifactQueries.some(
    (q) => q.isLoading,
  );
  const isLoading =
    isLoadingWorkUnits ||
    isLoadingWorkUnitArtifacts ||
    isLoadingTargetWorkUnit ||
    isLoadingTestResultArtifacts;
  const workUnits = useMemo(() => {
    const units = [...(workUnitsData?.workUnits || [])];
    if (units.length > 0 || workUnitsData) {
      const parentName =
        targetWorkUnitData?.parent ||
        (units.length > 0 ? units[units.length - 1].name : undefined);

      units.push(
        WorkUnit.fromPartial({
          name: targetWorkUnitName,
          workUnitId: workUnitId,
          parent: parentName,
          kind: targetWorkUnitData?.kind,
          moduleId: targetWorkUnitData?.moduleId,
          moduleShardKey: targetWorkUnitData?.moduleShardKey,
        }),
      );
    }
    return units;
  }, [workUnitsData, targetWorkUnitName, workUnitId, targetWorkUnitData]);

  if (workUnits.length === 0 && isLoading) {
    return <CircularProgress />;
  }

  return (
    <WorkUnitArtifactTree
      workUnits={workUnits}
      artifactsByWorkUnit={artifactsByWorkUnit}
      testResultArtifacts={testResultArtifactsData?.artifacts || []}
      selectedNodeId={searchParams.get('artifact')}
      isLoading={isLoading}
      onNodeSelect={(node) => {
        if (node) {
          setSelectedArtifact(node);
        }
      }}
    />
  );
}
