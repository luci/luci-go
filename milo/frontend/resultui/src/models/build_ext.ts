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

import {
  BLAMELIST_PIN_KEY,
  Build,
  BuilderID,
  BuildInfra,
  BuildInput,
  BuildOutput,
  BuildStatus,
  Executable,
  GitilesCommit,
  StringPair,
} from '../services/buildbucket';
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
  readonly canceledBy?: string;
  readonly createTime: DateTime;
  readonly startTime: DateTime | null;
  readonly endTime: DateTime | null;
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
    this.canceledBy = build.canceledBy;
    this.createTime = DateTime.fromISO(build.createTime);
    this.startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
    this.endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
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

  @computed
  get buildNumOrId(): string {
    return this.number?.toString() || 'b' + this.id;
  }

  @computed
  get isCanary(): boolean {
    return Boolean(this.input.experiments?.includes('luci.buildbucket.canary_software'));
  }

  @computed get buildSets(): string[] {
    return this.tags.filter((tag) => tag.key === 'buildset').map((tag) => tag.value);
  }

  @computed get associatedGitilesCommit() {
    return this.output.gitilesCommit || this.input.gitilesCommit;
  }

  @computed get blamelistPins(): GitilesCommit[] {
    const blamelistPins = this.output.properties[BLAMELIST_PIN_KEY] || [];
    if (blamelistPins.length === 0 && this.associatedGitilesCommit) {
      blamelistPins.push(this.associatedGitilesCommit);
    }
    return blamelistPins;
  }

  @computed get recipeLink(): Link {
    let csHost = 'source.chromium.org';
    if (this.exe.cipdPackage.includes('internal')) {
      csHost = 'source.corp.google.com';
    }
    // TODO(crbug.com/1149540): remove this conditional once the long-term
    // solution for recipe links has been implemented.
    if (this.builder.project === 'flutter') {
      csHost = 'cs.opensource.google';
    }
    const recipeName = this.input.properties['recipe'] as string;
    return {
      label: recipeName,
      url: `https://${csHost}/search/?${new URLSearchParams([['q', `file:recipes/${recipeName}.py`]]).toString()}`,
      ariaLabel: `recipe ${recipeName}`,
    };
  }

  @computed get pendingDuration(): Duration {
    return (this.startTime || this.renderTime.get()).diff(this.createTime);
  }

  @computed get executionDuration(): Duration | null {
    return this.startTime ? (this.endTime || this.renderTime.get()).diff(this.startTime) : null;
  }

  @computed get timeSinceCreated(): Duration {
    return this.renderTime.get().diff(this.createTime);
  }

  @computed get timeSinceStarted(): Duration | null {
    return this.startTime ? this.renderTime.get().diff(this.startTime) : null;
  }

  @computed get timeSinceEnded(): Duration | null {
    return this.endTime ? this.renderTime.get().diff(this.endTime) : null;
  }
}
