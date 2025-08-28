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

import { ReactNode, useEffect, useMemo, useState } from 'react';

import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  skippedReason_KindToJSON,
  TestResult_Status,
  testResult_StatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { normalizeFailureReason } from '@/test_investigation/utils/test_variant_utils';

import { ClusteredResult } from '../types';

import { ArtifactsContext, ArtifactsContextType } from './context';

// Clustering logic moved from ArtifactsSection
interface ClusterGroupData {
  results: TestResultBundle[];
  originalFailureReason: string;
  normalizedReasonKeyPart: string;
  failureKindKeyPart: FailureReason_Kind;
  skippedKindKeyPart: SkippedReason_Kind;
  statusV2KeyPart: TestResult_Status;
}

function getClusterSortPriority(status: TestResult_Status): number {
  switch (status) {
    case TestResult_Status.FAILED:
      return 1;
    case TestResult_Status.PASSED:
      return 2;
    case TestResult_Status.SKIPPED:
      return 3;
    case TestResult_Status.EXECUTION_ERRORED:
      return 4;
    case TestResult_Status.PRECLUDED:
      return 5;
    case TestResult_Status.STATUS_UNSPECIFIED:
    default:
      // Place unspecified or any new/unexpected statuses last.
      return 6;
  }
}

function clusterAndSortResults(
  results: readonly TestResultBundle[],
): ClusteredResult[] {
  const clusters = new Map<string, ClusterGroupData>();

  results.forEach((bundle: TestResultBundle) => {
    const reasonMsg = bundle.result?.failureReason?.primaryErrorMessage;
    const failureKind =
      bundle.result?.failureReason?.kind || FailureReason_Kind.KIND_UNSPECIFIED;
    const skippedKind =
      bundle.result?.skippedReason?.kind || SkippedReason_Kind.KIND_UNSPECIFIED;
    const statusV2 =
      bundle.result?.statusV2 || TestResult_Status.STATUS_UNSPECIFIED;

    const normalizedReason = normalizeFailureReason(reasonMsg);
    const failureKindStr = failureReason_KindToJSON(failureKind);
    const skippedKindStr = skippedReason_KindToJSON(skippedKind);
    const statusV2Str = testResult_StatusToJSON(statusV2);

    const key = `${statusV2Str}|${failureKindStr}|${skippedKindStr}|${normalizedReason}`;

    if (!clusters.has(key)) {
      clusters.set(key, {
        results: [],
        originalFailureReason: reasonMsg || '',
        normalizedReasonKeyPart: normalizedReason,
        failureKindKeyPart: failureKind,
        skippedKindKeyPart: skippedKind,
        statusV2KeyPart: statusV2,
      });
    }
    clusters.get(key)!.results.push(bundle);
  });

  const newClusteredFailures: ClusteredResult[] = Array.from(
    clusters.entries(),
  ).map(([key, data]) => ({
    clusterKey: key,
    results: data.results,
    normalizedReasonKeyPart: data.normalizedReasonKeyPart,
    failureKindKeyPart: data.failureKindKeyPart,
    skippedKindKeyPart: data.skippedKindKeyPart,
    statusV2KeyPart: data.statusV2KeyPart,
    originalFailureReason:
      data.originalFailureReason ||
      (key === 'Unknown' || key === 'No failure reason string specified.'
        ? 'No failure reason string specified.'
        : key),
  }));

  // Sort clusters by statusV2 according to the desired order.
  newClusteredFailures.sort(
    (a, b) =>
      getClusterSortPriority(a.statusV2KeyPart) -
      getClusterSortPriority(b.statusV2KeyPart),
  );

  return newClusteredFailures;
}

interface ArtifactsProviderProps {
  results: readonly TestResultBundle[];
  children: ReactNode;
}

export function ArtifactsProvider({
  results,
  children,
}: ArtifactsProviderProps) {
  const [selectedClusterIndex, setSelectedClusterIndex] = useState<number>(0);
  const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number>(0);

  const clusteredFailures = useMemo(
    () => clusterAndSortResults(results),
    [results],
  );

  useEffect(() => {
    setSelectedClusterIndex(0);
    setSelectedAttemptIndex(0);
  }, [clusteredFailures]);

  const currentCluster = useMemo(
    () => clusteredFailures[selectedClusterIndex],
    [clusteredFailures, selectedClusterIndex],
  );

  const currentAttempts = useMemo(
    () => currentCluster?.results || [],
    [currentCluster],
  );

  const currentAttemptBundle = useMemo(
    () => currentAttempts[selectedAttemptIndex],
    [currentAttempts, selectedAttemptIndex],
  );

  const currentResult = useMemo(
    () => currentAttemptBundle?.result,
    [currentAttemptBundle],
  );

  const handleSetSelectedClusterIndex = (index: number) => {
    setSelectedClusterIndex(index);
    setSelectedAttemptIndex(0); // Reset attempt index when cluster changes
  };

  const hasRenderableResults = results && results.length > 0;

  const value: ArtifactsContextType = {
    clusteredFailures,
    selectedClusterIndex,
    setSelectedClusterIndex: handleSetSelectedClusterIndex,
    selectedAttemptIndex,
    setSelectedAttemptIndex,
    currentCluster,
    currentAttempts,
    currentAttemptBundle,
    currentResult,
    hasRenderableResults,
  };

  return (
    <ArtifactsContext.Provider value={value}>
      {children}
    </ArtifactsContext.Provider>
  );
}
