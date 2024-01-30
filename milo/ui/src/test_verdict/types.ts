// Copyright 2023 The LUCI Authors.
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

/**
 * @fileoverview
 * Declare a list of types that are the same as the generated protobuf types
 * except that we also bake in the knowledge of which properties are always
 * defined when they are queried from the RPCs. This allows us to limit type
 * casting to only where the data is queried instead of having to use non-null
 * coercion (i.e. `object.nullable!`) everywhere.
 */

import { ChangepointGroupSummary } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { ClusterResponse_ClusteredTestResult_ClusterEntry } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  TestResultBundle,
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export type SpecifiedTestVerdictStatus = Exclude<
  TestVariantStatus,
  TestVariantStatus.UNSPECIFIED | TestVariantStatus.UNEXPECTED_MASK
>;

export type OutputTestResultBundle = NonNullableProps<
  TestResultBundle,
  'result'
>;

export type OutputTestVerdict = NonNullableProps<TestVariant, 'variant'> & {
  status: SpecifiedTestVerdictStatus;
  results: readonly OutputTestResultBundle[];
};

export type OutputClusterEntry = NonNullableProps<
  ClusterResponse_ClusteredTestResult_ClusterEntry,
  'clusterId'
>;

export type OutputChangepointGroupSummary = DeepNonNullableProps<
  ChangepointGroupSummary,
  'canonicalChangepoint' | 'statistics'
>;
