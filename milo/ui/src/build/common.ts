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

import { Status as BuildStatus } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export const BUILD_STATUS_DISPLAY_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'scheduled',
  [BuildStatus.STARTED]: 'running',
  [BuildStatus.SUCCESS]: 'succeeded',
  [BuildStatus.FAILURE]: 'failed',
  [BuildStatus.INFRA_FAILURE]: 'infra failed',
  [BuildStatus.CANCELED]: 'canceled',
});

export const BUILD_STATUS_CLASS_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'scheduled',
  [BuildStatus.STARTED]: 'started',
  [BuildStatus.SUCCESS]: 'success',
  [BuildStatus.FAILURE]: 'failure',
  [BuildStatus.INFRA_FAILURE]: 'infra-failure',
  [BuildStatus.CANCELED]: 'canceled',
});

export const BUILD_STATUS_ICON_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'schedule',
  [BuildStatus.STARTED]: 'pending',
  [BuildStatus.SUCCESS]: 'check_circle',
  [BuildStatus.FAILURE]: 'error',
  [BuildStatus.INFRA_FAILURE]: 'report',
  [BuildStatus.CANCELED]: 'cancel',
});

export const BUILD_STATUS_COLOR_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'var(--scheduled-color)',
  [BuildStatus.STARTED]: 'var(--started-color)',
  [BuildStatus.SUCCESS]: 'var(--success-color)',
  [BuildStatus.FAILURE]: 'var(--failure-color)',
  [BuildStatus.INFRA_FAILURE]: 'var(--critical-failure-color)',
  [BuildStatus.CANCELED]: 'var(--canceled-color)',
});

export const BUILD_STATUS_COLOR_THEME_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'scheduled',
  [BuildStatus.STARTED]: 'started',
  [BuildStatus.SUCCESS]: 'success',
  [BuildStatus.FAILURE]: 'error',
  [BuildStatus.INFRA_FAILURE]: 'criticalFailure',
  [BuildStatus.CANCELED]: 'canceled',
});
