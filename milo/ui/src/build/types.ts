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

import { ArrayElement, DeepNonNullableProps } from '@/generic_libs/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { Status as BuildStatus } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { Step } from '@/proto/go.chromium.org/luci/buildbucket/proto/step.pb';
import {
  Builder,
  Console,
} from '@/proto/go.chromium.org/luci/milo/proto/projectconfig/project.pb';
import {
  BuilderSnapshot,
  ConsoleSnapshot,
  QueryConsoleSnapshotsResponse,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { PARTIAL_BUILD_FIELD_MASK } from './constants';

export type SpecifiedBuildStatus = Exclude<
  BuildStatus,
  BuildStatus.STATUS_UNSPECIFIED | BuildStatus.ENDED_MASK
>;

export interface OutputStep extends Step {
  readonly status: SpecifiedBuildStatus;
}

export interface OutputBuild
  extends DeepNonNullableProps<Build, 'builder' | 'createTime'> {
  readonly status: SpecifiedBuildStatus;
  readonly steps: readonly OutputStep[];
}

export interface PartialBuild
  extends Pick<
    DeepNonNullableProps<Build, 'builder' | 'createTime'>,
    ArrayElement<typeof PARTIAL_BUILD_FIELD_MASK>
  > {
  readonly status: SpecifiedBuildStatus;
}

export interface OutputQueryConsoleSnapshotsResponse
  extends QueryConsoleSnapshotsResponse {
  readonly snapshots: readonly OutputConsoleSnapshot[];
}

export interface OutputConsoleSnapshot extends ConsoleSnapshot {
  readonly console: OutputConsole;
  readonly builderSnapshots: readonly OutputBuilderSnapshot[];
}

export interface OutputConsole extends Console {
  readonly builders: readonly OutputBuilder[];
}

export interface OutputBuilder extends Builder {
  readonly id: BuilderID;
}

export interface OutputBuilderSnapshot extends BuilderSnapshot {
  readonly builder: BuilderID;
  readonly build: OutputBuild | undefined;
}
