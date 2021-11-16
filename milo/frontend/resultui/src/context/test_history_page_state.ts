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

import { interpolateOranges, scaleLinear, scaleSequential } from 'd3';
import { DateTime } from 'luxon';
import { computed, observable, reaction } from 'mobx';

import { createContextLink } from '../libs/context';
import { TestHistoryLoader } from '../models/test_history_loader';
import { TestHistoryService } from '../services/test_history_service';

export const enum GraphType {
  STATUS = 'STATUS',
  DURATION = 'DURATION',
}

export const enum XAxisType {
  DATE = 'DATE',
  COMMIT = 'COMMIT',
}

// Use SCALE_COLOR to discard colors avoid using white color when the input is
// close to 0.
const SCALE_COLOR = scaleLinear().range([0.1, 1]).domain([0, 1]);

/**
 * Records the test history page state.
 */
export class TestHistoryPageState {
  readonly testHistoryLoader: TestHistoryLoader;
  readonly now = DateTime.now().startOf('day').plus({ hours: 12 });
  @observable.ref days = 14;

  @computed get endDate() {
    return this.now.minus({ days: this.days });
  }

  @computed get dates() {
    return Array(this.days)
      .fill(0)
      .map((_, i) => this.now.minus({ days: i }));
  }

  @observable.ref graphType = GraphType.STATUS;
  @observable.ref xAxisType = XAxisType.DATE;

  /**
   * Only include durations from passed results.
   */
  @observable.ref passOnlyDuration = true;

  @observable.ref countUnexpected = true;
  @observable.ref countUnexpectedlySkipped = true;
  @observable.ref countFlaky = true;

  // Keep track of the max and min duration to render the duration graph.
  @observable.ref durationInitialized = false;
  @observable.ref maxDurationMs = 100;
  @observable.ref minDurationMs = 0;

  resetDurations() {
    this.durationInitialized = false;
    this.maxDurationMs = 100;
    this.minDurationMs = 0;
  }

  @computed get scaleDurationColor() {
    return scaleSequential((x) => interpolateOranges(SCALE_COLOR(x))).domain([this.minDurationMs, this.maxDurationMs]);
  }

  private disposers: Array<() => void> = [];
  constructor(readonly realm: string, readonly testId: string, readonly testHistoryService: TestHistoryService) {
    this.testHistoryLoader = new TestHistoryLoader(
      realm,
      testId,
      (datetime) => datetime.toFormat('yyyy-MM-dd'),
      testHistoryService
    );

    // Ensure all the entries are loaded / being loaded.
    this.disposers.push(
      reaction(
        () => [this.testHistoryLoader, this.endDate],
        () => {
          if (!this.testHistoryLoader) {
            return;
          }
          this.testHistoryLoader.loadUntil(this.endDate);
        },
        { fireImmediately: true }
      )
    );
  }

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    for (const disposer of this.disposers) {
      disposer();
    }
  }
}

export const [provideTestHistoryPageState, consumeTestHistoryPageState] = createContextLink<TestHistoryPageState>();
