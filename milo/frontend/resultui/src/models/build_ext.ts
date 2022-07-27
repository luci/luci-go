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
import { computed, IObservableValue, makeObservable, observable } from 'mobx';

import { parseProtoDuration } from '../libs/time_utils';
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
  readonly cancelTime: DateTime | null;
  readonly status: BuildStatus;
  readonly summaryMarkdown?: string | undefined;
  readonly input?: BuildInput;
  readonly output?: BuildOutput;
  readonly steps: readonly StepExt[];
  readonly infra?: BuildInfra | undefined;
  readonly tags: readonly StringPair[];
  readonly exe: Executable;
  readonly schedulingTimeout: Duration | null;
  readonly executionTimeout: Duration | null;
  readonly gracePeriod: Duration | null;
  readonly ancestorIds: number[];

  readonly rootSteps: readonly StepExt[];
  private readonly renderTime: IObservableValue<DateTime>;

  constructor(build: Build, renderTime?: IObservableValue<DateTime>) {
    makeObservable(this);

    this.renderTime = renderTime || observable.box(DateTime.local());

    this.id = build.id;
    this.builder = build.builder;
    this.number = build.number;
    this.canceledBy = build.canceledBy;
    this.createTime = DateTime.fromISO(build.createTime);
    this.startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
    this.endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
    this.cancelTime = build.cancelTime ? DateTime.fromISO(build.cancelTime) : null;
    this.status = build.status;
    this.summaryMarkdown = build.summaryMarkdown;
    this.input = build.input;
    this.output = build.output;
    this.infra = build.infra;
    this.tags = build.tags;
    this.exe = build.exe;
    this.schedulingTimeout = build.schedulingTimeout
      ? Duration.fromMillis(parseProtoDuration(build.schedulingTimeout))
      : null;
    this.executionTimeout = build.executionTimeout
      ? Duration.fromMillis(parseProtoDuration(build.executionTimeout))
      : null;
    this.gracePeriod = build.gracePeriod ? Duration.fromMillis(parseProtoDuration(build.gracePeriod)) : null;
    this.ancestorIds = build.ancestorIds || [];

    // Build the step-tree.
    const steps: StepExt[] = [];
    const rootSteps: StepExt[] = [];
    const stepMap = new Map<string, StepExt>(); // Map step name -> StepExt.
    for (const step of build.steps || []) {
      const splitName = step.name.split('|');
      const selfName = splitName.pop()!;
      const depth = splitName.length;
      const parentName = splitName.join('|');
      const parent = stepMap.get(parentName);

      const index = (parent?.children || rootSteps).length;
      const stepExt = new StepExt({ step, depth, index, selfName, renderTime: this.renderTime });

      steps.push(stepExt);
      stepMap.set(step.name, stepExt);
      if (!parent) {
        rootSteps.push(stepExt);
      } else {
        parent.children.push(stepExt);
      }
    }
    this.steps = steps;
    this.rootSteps = rootSteps;
  }

  @computed
  get buildOrStepInfraFailed() {
    return this.status === BuildStatus.InfraFailure || this.steps.some((s) => s.status === BuildStatus.InfraFailure);
  }

  @computed
  get buildNumOrId(): string {
    return this.number?.toString() || 'b' + this.id;
  }

  @computed
  get isCanary(): boolean {
    return Boolean(this.input?.experiments?.includes('luci.buildbucket.canary_software'));
  }

  @computed get buildSets(): string[] {
    return this.tags.filter((tag) => tag.key === 'buildset').map((tag) => tag.value);
  }

  @computed get associatedGitilesCommit() {
    return this.output?.gitilesCommit || this.input?.gitilesCommit;
  }

  @computed get blamelistPins(): GitilesCommit[] {
    const blamelistPins = this.output?.properties?.[BLAMELIST_PIN_KEY] || [];
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
    const recipeName = this.input?.properties?.['recipe'] as string;
    return {
      label: recipeName,
      url: `https://${csHost}/search/?${new URLSearchParams([['q', `file:recipes/${recipeName}.py`]]).toString()}`,
      ariaLabel: `recipe ${recipeName}`,
    };
  }

  @computed get pendingDuration(): Duration {
    return (this.startTime || this.endTime || this.renderTime.get()).diff(this.createTime);
  }

  @computed get isPending(): boolean {
    return !this.startTime && !this.endTime;
  }

  /**
   * A build exceeded it's scheduling timeout when
   * * the build is canceled, AND
   * * the build did not enter the execution phase, AND
   * * the scheduling timeout is specified, AND
   * * the pending duration is no less than the scheduling timeout.
   */
  @computed get exceededSchedulingTimeout(): boolean {
    return (
      !this.startTime &&
      !this.isPending &&
      this.schedulingTimeout !== null &&
      this.pendingDuration >= this.schedulingTimeout
    );
  }

  @computed get executionDuration(): Duration | null {
    return this.startTime ? (this.endTime || this.renderTime.get()).diff(this.startTime) : null;
  }

  @computed get isExecuting(): boolean {
    return this.startTime !== null && !this.endTime;
  }

  /**
   * A build exceeded it's execution timeout when
   * * the build is canceled, AND
   * * the build had entered the execution phase, AND
   * * the execution timeout is specified, AND
   * * the execution duration is no less than the execution timeout.
   */
  @computed get exceededExecutionTimeout(): boolean {
    return (
      this.status === BuildStatus.Canceled &&
      this.executionDuration !== null &&
      this.executionTimeout !== null &&
      this.executionDuration >= this.executionTimeout
    );
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
