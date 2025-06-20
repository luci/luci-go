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

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { FailureReason_Kind } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
// Import TestResultBundle instead of TestResultLink
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export interface CustomArtifactTreeNode {
  id: string;
  name: string;
  children?: CustomArtifactTreeNode[];
  isSummary?: boolean;
  isLeaf?: boolean;
  artifact?: Artifact;
  level: number;
  isRootChild?: boolean;
}

export interface ClusteredResult {
  clusterKey: string; // The composite key: statusV2Str|failureKindStr|skippedKindStr|normalizedReason
  results: TestResultBundle[];
  originalFailureReason: string;
  normalizedReasonKeyPart: string;
  failureKindKeyPart: FailureReason_Kind;
  skippedKindKeyPart: SkippedReason_Kind;
  statusV2KeyPart: TestResult_Status;
}

export interface FetchedArtifactContent {
  data: string | null;
  isText: boolean;
  contentType: string | null;
  error: Error | null;
  sizeBytes?: number | string;
}
