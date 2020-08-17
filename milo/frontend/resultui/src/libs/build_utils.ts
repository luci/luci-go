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

import { router } from '../routes';
import { Build, BuildStatus } from '../services/buildbucket';

export function getURLForBuild(build: Build): string {
  return router.urlForName(
      'build',
      {
      'project': build.builder.project,
      'bucket': build.builder.bucket,
      'builder': build.builder.builder,
      'build_num_or_id': 'b' + build.id,
      },
  );
}

export function getURLForBuilder(build: Build): string {
  return `/p/${build.builder.project}/builders/${build.builder.bucket}/${build.builder.builder}`
}

export function getDisplayNameForStatus(s: BuildStatus): string {
  const statusMap = new Map([
    [BuildStatus.Scheduled, "Scheduled"],
    [BuildStatus.Started, "Running"],
    [BuildStatus.Success, "Success"],
    [BuildStatus.Failure, "Failure"],
    [BuildStatus.InfraFailure, "Infra Failure"],
    [BuildStatus.Canceled, "Canceled"],
  ]);
  return statusMap.get(s) || "Unknown";
}