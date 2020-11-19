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

// tslint:disable: variable-name

import { DateTime } from 'luxon';
import { computed, IObservableValue, observable } from 'mobx';

import { BuildStatus, Log, Step } from '../services/buildbucket';

/**
 * Contains all fields of the Step object with added helper methods and
 * properties.
 */
export class StepExt {
  readonly name: string;
  readonly startTime: DateTime;
  readonly endTime: DateTime | null;
  readonly status: BuildStatus;
  readonly logs?: Log[] | undefined;
  readonly summaryMarkdown?: string | undefined;

  readonly selfName: string;
  readonly parentName: string | null;
  readonly children: StepExt[] = [];
  readonly renderTime: IObservableValue<DateTime>;

  constructor(step: Step, renderTime?: IObservableValue<DateTime>) {
    if (!step.name) {
      throw new Error('Step name can not be empty');
    }

    this.name = step.name;
    this.startTime = DateTime.fromISO(step.startTime);
    this.endTime = step.endTime ? DateTime.fromISO(step.endTime): null;
    this.status = step.status;
    this.logs = step.logs;
    this.summaryMarkdown = step.summaryMarkdown;

    const lastSeparatorIndex = step.name.lastIndexOf('|');
    this.selfName = step.name.slice(lastSeparatorIndex + 1);
    this.parentName = lastSeparatorIndex === -1 ? null : step.name.slice(0, lastSeparatorIndex);

    this.renderTime = renderTime || observable.box(DateTime.local());
  }

  /**
   * true if and only if the step and all of its descendants succeeded.
   */
  @computed get succeededRecursively(): boolean {
    if (this.status !== BuildStatus.Success) {
      return false;
    }
    return this.children.every((child) => child.succeededRecursively);
  }

  /**
   * true iff the step or one of its descendants failed (status Failure or InfraFailure).
   */
  @computed get failed(): boolean {
      if (this.status === BuildStatus.Failure ||
          this.status === BuildStatus.InfraFailure) {
          return true;
      }
      return this.children.some((child) => child.failed);
  }

  @computed get duration() {
    return (this.endTime || this.renderTime.get()).diff(this.startTime);
  }

  @computed get header() {
    return this.summaryMarkdown?.split(/\<br\/?\>/i)[0] || '';
  }

  @computed get summary() {
    return this.summaryMarkdown?.split(/\<br\/?\>/i).slice(1).join('<br>') || '';
  }
}
