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

import { taskResultTone } from './task_result_tone';

describe('taskResultTone', () => {
  it.each([
    ['COMPLETED', 'success', 'Completed'],
    ['SUCCESS', 'success', 'Passed'],
    ['TIMED_OUT', 'warning', 'Timed out'],
    ['FAILURE', 'error', 'Failure'],
    ['CLIENT_ERROR', 'error', 'Client error'],
    ['RUNNING', 'info', 'Running'],
    ['PENDING', 'info', 'Pending'],
    ['BOT_DIED', 'warning', 'Bot died'],
    ['INFRA_FAILURE', 'warning', 'Infra failure'],
    ['NOT_RUN', 'neutral', 'Not run'],
    ['UNSUCCESSFUL', 'neutral', 'Unsuccessful'],
    ['CANCELED', 'neutral', 'Canceled'],
  ])('maps %s to %s / %s', (raw, tone, label) => {
    expect(taskResultTone(raw)).toEqual({ tone, label });
  });
});
