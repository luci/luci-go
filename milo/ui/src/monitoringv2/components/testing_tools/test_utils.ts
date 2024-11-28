// Copyright 2024 The LUCI Authors.
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

import {
  BuilderAlert,
  StepAlert,
} from '@/monitoringv2/pages/monitoring_page/context/context';
import { builderPath } from '@/monitoringv2/util/server_json';

import { organizeAlerts, StructuredAlert } from '../alerts/alert_tabs';

const builderID = {
  project: 'chromium',
  bucket: 'ci',
  builder: 'linux-rel',
};
export const testBuilderAlert: BuilderAlert = {
  kind: 'builder',
  key: builderPath(builderID),
  builderID: builderID,
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testStepAlert: StepAlert = {
  kind: 'step',
  key: `${builderPath(builderID)}/compile`,
  builderID: builderID,
  stepName: 'compile',
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testAlerts: StructuredAlert[] = organizeAlerts(
  'builder',
  [testBuilderAlert],
  [testStepAlert],
  [],
);

const builderID2 = {
  project: 'chromium',
  bucket: 'ci',
  builder: 'win-rel',
};

export const testBuilderAlert2: BuilderAlert = {
  kind: 'builder',
  key: builderPath(builderID2),
  builderID: builderID2,
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testAlerts2: StructuredAlert[] = organizeAlerts(
  'builder',
  [testBuilderAlert2],
  [],
  [],
);
