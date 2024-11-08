// Copyright 2022 The LUCI Authors.
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

import { DateTime } from 'luxon';

import { ReclusteringProgress } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

export const createMockProgress = (progress: number): ReclusteringProgress => {
  return {
    name: 'projects/testproject/reclusteringProgress',
    progressPerMille: progress,
    next: {
      rulesVersion: DateTime.now().toISO(),
      configVersion: DateTime.now().toISO(),
      algorithmsVersion: 7,
    },
    last: {
      rulesVersion: DateTime.now()
        .minus({
          minute: 2,
        })
        .toISO(),
      configVersion: DateTime.now()
        .minus({
          minute: 2,
        })
        .toISO(),
      algorithmsVersion: 6,
    },
  };
};

export const createMockDoneProgress = (): ReclusteringProgress => {
  const currentDate = DateTime.now();
  return {
    name: 'projects/testproject/reclusteringProgress',
    progressPerMille: 1000,
    next: {
      rulesVersion: currentDate.toISO(),
      configVersion: currentDate.toISO(),
      algorithmsVersion: 7,
    },
    last: {
      rulesVersion: currentDate.toISO(),
      configVersion: currentDate.toISO(),
      algorithmsVersion: 7,
    },
  };
};
