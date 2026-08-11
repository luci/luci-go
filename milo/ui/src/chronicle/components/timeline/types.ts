// Copyright 2026 The LUCI Authors.
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

import { StageResultStatus } from '@/chronicle/utils/check_utils';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';

export interface TimelineItem {
  id: string;
  label: string;
  start: DateTime;
  end: DateTime;
  stage: Stage;
  resultStatus: StageResultStatus;
}

export const ROW_HEIGHT = 30;
export const BAR_HEIGHT = 24;
export const STAGE_COLUMN_WIDTH = 300;

export const SELECTED_BAR_STYLE = {
  fill: '#e3f2fd',
  stroke: '#1976d2',
  color: '#1976d2',
};
