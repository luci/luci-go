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

import { useInfiniteQuery, useQueries, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  ArtifactPredicate,
  ArtifactPredicate_ArtifactKind as ArtifactKind,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { QueryArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  BatchGetWorkUnitsRequest,
  GetWorkUnitRequest,
  WorkUnit,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';

/**
 * Recursively fetches work units starting from the root work unit of the given root invocation.
 * The recursion stops at work units that have a module_id set.
 */
export function useInvocationWorkUnitTree(rootInvocationId: string) {
  const client = useResultDbClient();
  const rootWorkUnitName = `rootInvocations/${rootInvocationId}/workUnits/root`;
  const { data: rootWorkUnit, isLoading: isLoadingRoot } = useQuery(
    client.GetWorkUnit.query(
      GetWorkUnitRequest.fromPartial({
        name: rootWorkUnitName,
      }),
    ),
  );

  const [fetchedWorkUnits, setFetchedWorkUnits] = useState<
    Record<string, WorkUnit>
  >({});

  // Track which work units we have found that need expansion (have children and no module_id)
  const [expandQueue, setExpandQueue] = useState<string[]>([]);

  // Persistent list of batches to query (prevents queries from being cancelled)
  const [batches, setBatches] = useState<{ childNames: string[] }[]>([]);

  // Track which ones we have already processed (sent requests for)
  const processedSet = useRef(new Set<string>());

  useEffect(() => {
    if (rootWorkUnit) {
      setFetchedWorkUnits((prev) => ({
        ...prev,
        [rootWorkUnit.name]: rootWorkUnit,
      }));

      const shouldExpand =
        rootWorkUnit.childWorkUnits?.length > 0 && !rootWorkUnit.moduleId;

      if (shouldExpand && !processedSet.current.has(rootWorkUnit.name)) {
        setExpandQueue((prev) => {
          if (prev.includes(rootWorkUnit.name)) return prev;
          return [...prev, rootWorkUnit.name];
        });
      }
    }
  }, [rootWorkUnit]);

  useEffect(() => {
    const allNewChildren: string[] = [];
    const processedParents = new Set<string>();

    for (const parentName of expandQueue) {
      if (processedSet.current.has(parentName)) continue;

      // Mark as processed regardless of validity to clear queue
      processedParents.add(parentName);

      const parent = fetchedWorkUnits[parentName];
      if (!parent || !parent.childWorkUnits?.length) {
        continue;
      }

      allNewChildren.push(...parent.childWorkUnits);
    }

    if (processedParents.size > 0) {
      processedParents.forEach((p) => processedSet.current.add(p));

      // Chunk requests into groups of 500 across ALL parents
      const newBatches: { childNames: string[] }[] = [];
      for (let i = 0; i < allNewChildren.length; i += 500) {
        newBatches.push({
          childNames: allNewChildren.slice(i, i + 500),
        });
      }

      if (newBatches.length > 0) {
        setBatches((prev) => [...prev, ...newBatches]);
      }

      setExpandQueue((prev) =>
        prev.filter((p) => !processedSet.current.has(p)),
      );
    }
  }, [expandQueue, fetchedWorkUnits]);

  const batchQueries = useQueries({
    queries: batches.map((batch) => ({
      ...client.BatchGetWorkUnits.query(
        BatchGetWorkUnitsRequest.fromPartial({
          parent: `rootInvocations/${rootInvocationId}`,
          names: batch.childNames,
        }),
      ),
      enabled: true,
    })),
  });

  useEffect(() => {
    const newWorkUnits: Record<string, WorkUnit> = {};
    const newExpandCandidates: string[] = [];

    let hasNew = false;
    batchQueries.forEach((query) => {
      if (query.data && query.data.workUnits) {
        for (const wu of query.data.workUnits) {
          if (!fetchedWorkUnits[wu.name]) {
            newWorkUnits[wu.name] = wu;
            hasNew = true;
            if (wu.childWorkUnits?.length > 0 && !wu.moduleId) {
              newExpandCandidates.push(wu.name);
            }
          }
        }
      }
    });

    if (hasNew) {
      setFetchedWorkUnits((prev) => ({ ...prev, ...newWorkUnits }));
      setExpandQueue((prev) => {
        const next = [...prev];
        newExpandCandidates.forEach((c) => {
          if (!next.includes(c) && !processedSet.current.has(c)) {
            next.push(c);
          }
        });
        return next;
      });
    }
  }, [batchQueries, fetchedWorkUnits]);

  const isLoading =
    isLoadingRoot ||
    batchQueries.some((q) => q.isLoading) ||
    expandQueue.length > 0;

  const workUnits = useMemo(
    () => Object.values(fetchedWorkUnits),
    [fetchedWorkUnits],
  );

  return {
    workUnits,
    isLoading,
  };
}

export function useAllInvocationArtifacts(rootInvocationId: string) {
  const resultDbClient = useResultDbClient();

  return useInfiniteQuery({
    ...resultDbClient.QueryArtifacts.queryPaged(
      QueryArtifactsRequest.fromPartial({
        parent: `rootInvocations/${rootInvocationId}`,
        pageSize: 1000,
        readMask: [
          'name',
          'artifact_id',
          'content_type',
          'size_bytes',
          'has_lines',
          'artifact_type',
        ],
        predicate: ArtifactPredicate.fromPartial({
          artifactKind: ArtifactKind.WORK_UNIT,
        }),
      }),
    ),
    enabled: !!rootInvocationId,
    refetchOnWindowFocus: false,
    staleTime: Infinity,
  });
}
