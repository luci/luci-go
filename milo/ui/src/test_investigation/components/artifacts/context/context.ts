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

import { createContext, useContext } from 'react';

import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { ArtifactTreeNodeData, ClusteredResult } from '../types';

export interface ArtifactsContextType {
  clusteredFailures: ClusteredResult[];
  selectedClusterIndex: number;
  setSelectedClusterIndex: (index: number) => void;
  selectedAttemptIndex: number;
  setSelectedAttemptIndex: (index: number) => void;
  currentCluster: ClusteredResult | undefined;
  currentAttempts: readonly TestResultBundle[];
  currentAttemptBundle: TestResultBundle | undefined;
  currentResult: TestResult | undefined;
  hasRenderableResults: boolean;
  selectedArtifact: ArtifactTreeNodeData | null;
  setSelectedArtifact: (node: ArtifactTreeNodeData | null) => void;
}

export const ArtifactsContext = createContext<ArtifactsContextType | null>(
  null,
);

export function useArtifactsContext() {
  const context = useContext(ArtifactsContext);
  if (!context) {
    throw new Error(
      'useArtifactsContext must be used within an ArtifactsProvider',
    );
  }
  return context;
}
