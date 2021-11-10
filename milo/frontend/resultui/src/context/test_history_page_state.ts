// Copyright 2021 The LUCI Authors.
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

import { observable } from 'mobx';

import { createContextLink } from '../libs/context';

export const enum GraphType {
  STATUS = 'STATUS',
  DURATION = 'DURATION',
}

export const enum XAxisType {
  DATE = 'DATE',
  COMMIT = 'COMMIT',
}

/**
 * Records the test history page state.
 */
export class TestHistoryPageState {
  @observable.ref graphType = GraphType.STATUS;
  @observable.ref xAxisType = XAxisType.DATE;

  @observable.ref countUnexpected = true;
  @observable.ref countUnexpectedlySkipped = true;
  @observable.ref countFlaky = true;
}

export const [provideTestHistoryPageState, consumeTestHistoryPageState] = createContextLink<TestHistoryPageState>();
