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

import dayjs from 'dayjs';

import { ReclusteringProgress } from '@/legacy_services/cluster';

export const createMockProgress = (progress: number): ReclusteringProgress => {
  return {
    progressPerMille: progress,
    next: {
      rulesVersion: dayjs().toISOString(),
      configVersion: dayjs().toISOString(),
      algorithmsVersion: 7,
    },
    last: {
      rulesVersion: dayjs().subtract(2, 'minutes').toISOString(),
      configVersion: dayjs().subtract(2, 'minutes').toISOString(),
      algorithmsVersion: 6,
    },
  };
};

export const createMockDoneProgress = (): ReclusteringProgress => {
  const currentDate = dayjs();
  return {
    progressPerMille: 1000,
    next: {
      rulesVersion: currentDate.toISOString(),
      configVersion: currentDate.toISOString(),
      algorithmsVersion: 7,
    },
    last: {
      rulesVersion: currentDate.toISOString(),
      configVersion: currentDate.toISOString(),
      algorithmsVersion: 7,
    },
  };
};
