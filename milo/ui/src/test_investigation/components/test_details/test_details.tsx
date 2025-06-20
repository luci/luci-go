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

import { Box, SelectChangeEvent, Typography } from '@mui/material';
import { useEffect, useState } from 'react';

import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  skippedReason_KindToJSON,
  TestResult,
  TestResult_Status,
  testResult_StatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

import { normalizeFailureReason } from '../../utils/test_variant_utils';

import { ArtifactsSection } from './artifacts/artifacts_section';
import { ClusteringControls } from './clustering_controls';
import { TagsSection } from './tags_section';
import { ClusteredResult } from './types';

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

export function TestDetails() {
  const invocation = useInvocation();
  const testVariant = useTestVariant();

  const [clusteredFailures, setClusteredFailures] = useState<ClusteredResult[]>(
    [],
  );
  const [selectedClusterIndex, setSelectedClusterIndex] = useState<number>(0);
  const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number>(0);

  useEffect(() => {
    const clusters = new Map<string, ClusterGroupData>();

    testVariant.results.forEach((bundle: TestResultBundle) => {
      const reasonMsg = bundle.result?.failureReason?.primaryErrorMessage;
      const failureKind =
        bundle.result?.failureReason?.kind ||
        FailureReason_Kind.KIND_UNSPECIFIED;
      const skippedKind =
        bundle.result?.skippedReason?.kind ||
        SkippedReason_Kind.KIND_UNSPECIFIED;
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

    setClusteredFailures(newClusteredFailures);
    setSelectedClusterIndex(0);
    setSelectedAttemptIndex(0);
  }, [testVariant.results]);

  const currentCluster: ClusteredResult | undefined =
    clusteredFailures[selectedClusterIndex];
  const currentAttempts: TestResultBundle[] = currentCluster?.results || [];
  const currentAttemptBundle: TestResultBundle | undefined =
    currentAttempts[selectedAttemptIndex];
  const currentResult: TestResult | undefined = currentAttemptBundle?.result;

  const handleClusterChange = (event: SelectChangeEvent<number>) => {
    setSelectedClusterIndex(Number(event.target.value));
    setSelectedAttemptIndex(0);
  };

  const handleAttemptChange = (event: SelectChangeEvent<number>) => {
    setSelectedAttemptIndex(Number(event.target.value));
  };

  const tagsPanelId = 'test-tab-tags-content';
  const tagsHeaderId = 'test-tab-tags-header';
  const artifactsPanelId = 'test-tab-artifacts-content';
  const artifactsHeaderId = 'test-tab-artifacts-header';

  const hasRenderableResults =
    testVariant.results && testVariant.results.length > 0;
  const noFailuresToClusterMessage =
    'No failures to cluster based on v2 status (FAILED or EXECUTION_ERRORED).';

  return (
    <>
      {clusteredFailures.length > 0 && currentCluster ? (
        <ClusteringControls
          clusteredFailures={clusteredFailures}
          selectedClusterIndex={selectedClusterIndex}
          onClusterChange={handleClusterChange}
          currentAttempts={currentAttempts}
          selectedAttemptIndex={selectedAttemptIndex}
          onAttemptChange={handleAttemptChange}
          currentCluster={currentCluster}
        />
      ) : (
        hasRenderableResults && (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {noFailuresToClusterMessage}
          </Typography>
        )
      )}

      <Box sx={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        <TagsSection
          tags={currentResult?.tags}
          panelId={tagsPanelId}
          headerId={tagsHeaderId}
        />
        {currentResult && (
          <ArtifactsSection
            currentResult={currentResult}
            invocationName={invocation.name}
            panelId={artifactsPanelId}
            headerId={artifactsHeaderId}
          />
        )}
      </Box>
    </>
  );
}
