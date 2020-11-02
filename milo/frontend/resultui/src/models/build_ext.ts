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

import { DateTime, Duration } from 'luxon';
import { computed, IObservableValue, observable } from 'mobx';

import { Build, BuilderID, BuildInfra, BuildInput, BuildOutput, BuildStatus, Executable, StringPair } from '../services/buildbucket';
import { Link } from './link';
import { StepExt } from './step_ext';

/**
 * Contains all fields of the Build object with added helper methods and
 * properties.
 */
export class BuildExt {
  readonly id: string;
  readonly builder: BuilderID;
  readonly number?: number;
  readonly createdBy: string;
  readonly canceledBy?: string;
  readonly createTime: DateTime;
  readonly startTime: DateTime | null;
  readonly endTime: DateTime | null;
  readonly updateTime: DateTime;
  readonly status: BuildStatus;
  readonly summaryMarkdown?: string | undefined;
  readonly input: BuildInput;
  readonly output: BuildOutput;
  readonly steps: readonly StepExt[];
  readonly infra?: BuildInfra | undefined;
  readonly tags: readonly StringPair[];
  readonly exe: Executable;

  readonly rootSteps: readonly StepExt[];
  private readonly renderTime: IObservableValue<DateTime>;

  constructor(build: Build, renderTime?: IObservableValue<DateTime>) {
    this.renderTime = renderTime || observable.box(DateTime.local());

    this.id = build.id;
    this.builder = build.builder;
    this.number = build.number;
    this.createdBy = build.createdBy;
    this.canceledBy = build.canceledBy;
    this.createTime = DateTime.fromISO(build.createTime);
    this.startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
    this.endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
    this.updateTime = DateTime.fromISO(build.updateTime);
    this.status = build.status;
    this.summaryMarkdown = build.summaryMarkdown;
    this.input = build.input;
    this.output = build.output;
    this.steps = build.steps?.map((s) => new StepExt(s, this.renderTime)) || [];
    this.infra = build.infra;
    this.tags = build.tags;
    this.exe = build.exe;

    // Build the step-tree.
    const rootSteps: StepExt[] = [];
    const stepMap = new Map<string, StepExt>();
    for (const step of this.steps) {
      stepMap.set(step.name, step);
      if (step.parentName === null) {
        rootSteps.push(step);
      } else {
        stepMap.get(step.parentName)!.children.push(step);
      }
    }
    this.rootSteps = rootSteps;
  }

  @computed get buildSets(): string[] {
    return this.tags
      .filter((tag) => tag.key === 'buildset')
      .map((tag) => tag.value);
  }

  @computed get recipeLink(): Link {
    const csHost = this.exe.cipdPackage.includes('internal')
      ? 'source.corp.google.com'
      : 'source.chromium.org';
    const recipeName = this.input.properties['recipe'] as string;
    return {
      label: recipeName,
      url: `https://${csHost}/search/?${new URLSearchParams([['q', `file:recipes/${recipeName}.py`]]).toString()}`,
      ariaLabel: `recipe ${recipeName}`,
    };
  }

  @computed get summary(): string[] {
    const summary: string[] = [];
    for (const step of this.steps) {
      const name = step.name.split('|').pop();
      if (name === 'Failure reason') {
        continue;
      }
      if (step.status === BuildStatus.InfraFailure) {
        summary.push(`Infra Failure ${step.name}`);
      }
      if (step.status === BuildStatus.Failure) {
        summary.push(`Failure ${step.name}`);
      }
    }
    return summary;
  }

  @computed get pendingDuration(): Duration {
    return (this.startTime || this.renderTime.get()).diff(this.createTime);
  }

  @computed get executionDuration(): Duration | null {
    return this.startTime ? (this.endTime || this.renderTime.get()).diff(this.startTime) : null;
  }
}
