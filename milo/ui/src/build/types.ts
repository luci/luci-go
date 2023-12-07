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

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { Status as BuildStatus } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export type SpecifiedBuildStatus = Exclude<
  BuildStatus,
  BuildStatus.UNSPECIFIED | BuildStatus.ENDED_MASK
>;

export type OutputBuild = DeepNonNullableProps<
  Build & { status: SpecifiedBuildStatus },
  'builder' | 'createTime'
>;
