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
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

import { normalizeFailureReason } from '../../utils/test_variant_utils';

import { ArtifactsSection } from './artifacts_section';
import { ClusteringControls } from './clustering_controls';
import { TagsSection } from './tags_section';
import { ClusteredResult } from './types';

export function TestTabContent() {
  const invocation = useInvocation();
  const testVariant = useTestVariant();

  const [clusteredFailures, setClusteredFailures] = useState<ClusteredResult[]>(
    [],
  );
  const [selectedClusterIndex, setSelectedClusterIndex] = useState<number>(0);
  const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number>(0);

  useEffect(() => {
    const failedResultBundles: TestResultBundle[] =
      testVariant.results?.filter((bundle: TestResultBundle) => {
        if (!bundle.result) return false;
        // A result is considered a "failure for clustering" if its statusV2
        // is FAILED or EXECUTION_ERRORED.
        return (
          bundle.result.statusV2 === TestResult_Status.FAILED ||
          bundle.result.statusV2 === TestResult_Status.EXECUTION_ERRORED
        );
      }) || [];

    const clusters = new Map<
      string,
      { results: TestResultBundle[]; originalFailureReason?: string }
    >();
    failedResultBundles.forEach((bundle: TestResultBundle) => {
      const reasonMsg = bundle.result?.failureReason?.primaryErrorMessage;
      const key = normalizeFailureReason(reasonMsg);
      if (!clusters.has(key)) {
        clusters.set(key, { results: [], originalFailureReason: reasonMsg });
      }
      clusters.get(key)!.results.push(bundle);
    });
    const newClusteredFailures: ClusteredResult[] = Array.from(
      clusters.entries(),
    ).map(([key, data]) => ({
      clusterKey: key,
      results: data.results,
      originalFailureReason:
        data.originalFailureReason ||
        (key === 'Unknown' || key === 'No failure reason string specified.'
          ? 'No failure reason string specified.'
          : key),
    }));

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
          currentResult={currentResult}
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
        <ArtifactsSection
          currentResult={currentResult}
          invocationName={invocation.name}
          panelId={artifactsPanelId}
          headerId={artifactsHeaderId}
        />
      </Box>
    </>
  );
}
