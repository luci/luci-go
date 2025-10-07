// Copyright 2020 The LUCI Authors.
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

import { GitilesCommit, StringPair } from '@/common/services/common';

/**
 * Manually coded type definition and classes for buildbucket services.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source:                  https://chromium.googlesource.com/infra/luci/luci-go/+/04a118946d13ad326c44dba9a635116ff7f31c4e/buildbucket/proto/builds_service.proto
 * Builder metadata source: https://chromium.googlesource.com/infra/luci/luci-go/+/fe56f864b0e1dc61eaa6b9062fabb1119e872306/buildbucket/proto/builder_service.proto
 */
export const TEST_PRESENTATION_KEY =
  '$recipe_engine/resultdb/test_presentation';

export const enum Trinary {
  Unset = 'UNSET',
  Yes = 'YES',
  No = 'NO',
}

export interface BuilderID {
  readonly project: string;
  readonly bucket: string;
  readonly builder: string;
}

export interface Build {
  readonly id: string;
  readonly builder: BuilderID;
  readonly number?: number;
  readonly status: BuildbucketStatus;
  readonly input?: BuildInput;
  readonly output?: BuildOutput;
  readonly steps?: readonly Step[];
  readonly infra?: BuildInfra;
  readonly tags?: readonly StringPair[];
}

// This is from https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/buildbucket/proto/common.proto#25
export enum BuildbucketStatus {
  Scheduled = 'SCHEDULED',
  Started = 'STARTED',
  Success = 'SUCCESS',
  Failure = 'FAILURE',
  InfraFailure = 'INFRA_FAILURE',
  Canceled = 'CANCELED',
}

export interface TestPresentationConfig {
  /**
   * A list of keys that will be rendered as columns in the test results tab.
   * status is always the first column and name is always the last column (you
   * don't need to specify them).
   *
   * A key must be one of the following:
   * 1. 'v.{variant_key}': variant.def[variant_key] of the test variant (e.g.
   * v.gpu).
   */
  column_keys?: string[];
  /**
   * A list of keys that will be used for grouping test variants in the test
   * results tab.
   *
   * A key must be one of the following:
   * 1. 'status': status of the test variant.
   * 2. 'name': test_metadata.name of the test variant.
   * 3. 'v.{variant_key}': variant.def[variant_key] of the test variant (e.g.
   * v.gpu).
   *
   * Caveat: test variants with only expected results are not affected by this
   * setting and are always in their own group.
   */
  grouping_keys?: string[];
}

export interface BuildInput {
  readonly properties?: {
    [TEST_PRESENTATION_KEY]?: TestPresentationConfig;
    [key: string]: unknown;
  };
}

export interface BuildOutput {
  readonly properties?: {
    [TEST_PRESENTATION_KEY]?: TestPresentationConfig;
    [key: string]: unknown;
  };
  readonly gitilesCommit?: GitilesCommit;
  readonly logs: Log[];
}

export interface Log {
  readonly name: string;
  readonly viewUrl: string;
  readonly url: string;
}

export interface Step {
  readonly name: string;
  readonly startTime?: string;
  readonly endTime?: string;
  readonly status: BuildbucketStatus;
  readonly logs?: Log[];
  readonly summaryMarkdown?: string;
  readonly tags?: readonly StringPair[];
}

export interface BuildInfra {
  readonly resultdb?: BuildInfraResultdb;
}

export interface BuildInfraResultdb {
  readonly invocation?: string;
}
