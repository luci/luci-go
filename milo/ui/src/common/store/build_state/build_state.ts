// Copyright 2022 The LUCI Authors.
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

import { render } from 'lit';
// We have to use `unsafeHTML` here because we are rendering into a detached
// HTML node to extract some HTML snippet. There's no `<LitReactBridge />` to
// support `<milo-sanitized-html />`.
// eslint-disable-next-line no-restricted-imports
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { DateTime, Duration } from 'luxon';
import { action, computed, makeObservable, untracked } from 'mobx';
import { Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import {
  Build,
  BuildbucketStatus,
  Log,
  Step,
} from '@/common/services/buildbucket';
import { StringPair } from '@/common/services/common';
import { Timestamp, TimestampInstance } from '@/common/store/timestamp';
import { UserConfig, UserConfigInstance } from '@/common/store/user_config';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { sanitizeHTML } from '@/common/tools/sanitize_html';
import { keepAliveComputed } from '@/generic_libs/tools/mobx_utils';

export interface StepInit {
  step: Step;
  depth: number;
  index: number;
  selfName: string;
  listNumber: string;
  userConfig?: UserConfigInstance;
  currentTime?: TimestampInstance;
}

/**
 * Contains all fields of the Step object with added helper methods and
 * properties.
 */
export class StepExt {
  readonly name: string;
  readonly startTime: DateTime | null;
  readonly endTime: DateTime | null;
  readonly status: BuildbucketStatus;
  readonly logs: readonly Log[];
  readonly summaryMarkdown?: string | undefined;
  readonly tags: readonly StringPair[];

  readonly depth: number;
  readonly index: number;
  readonly selfName: string;
  readonly listNumber: string;
  readonly children: StepExt[] = [];

  readonly userConfig?: UserConfigInstance;
  readonly currentTime?: TimestampInstance;

  constructor(init: StepInit) {
    makeObservable(this);
    this.userConfig = init.userConfig;

    const step = init.step;
    this.name = step.name;
    this.startTime = step.startTime ? DateTime.fromISO(step.startTime) : null;
    this.endTime = step.endTime ? DateTime.fromISO(step.endTime) : null;
    this.status = step.status;
    this.logs = step.logs || [];
    this.summaryMarkdown = step.summaryMarkdown;
    this.tags = step.tags || [];

    this.depth = init.depth;
    this.index = init.index;
    this.selfName = init.selfName;
    this.listNumber = init.listNumber;
    this.currentTime = init.currentTime;
  }

  @computed private get _currentTime() {
    return this.currentTime?.dateTime || DateTime.now();
  }

  @computed get duration() {
    if (!this.startTime) {
      return Duration.fromMillis(0);
    }
    return (this.endTime || this._currentTime).diff(this.startTime);
  }

  /**
   * summary renders summaryMarkdown into HTML.
   */
  // TODO(weiweilin): we should move this to build_step.ts because it contains
  // rendering logic.
  @computed get summary() {
    const bodyContainer = document.createElement('div');
    render(
      // We have to use `unsafeHTML` here because we are rendering into a
      // detached HTML node to extract some HTML snippet. There's no
      // `<ReactLitBridge />` to support `<milo-sanitized-html />`.
      // Sanitize manually. Note that it should've been sanitized automatically
      // by the trusted type policy in prod. But `sanitizeHTML` adds another
      // layer of protection in case CSP is not configured properly.
      unsafeHTML(sanitizeHTML(renderMarkdown(this.summaryMarkdown || ''))),
      bodyContainer,
    );
    // The body has no content.
    // We don't need to check bodyContainer.firstChild because text are
    // automatically wrapped in <p>.
    if (bodyContainer.firstElementChild === null) {
      return null;
    }

    // Show a tooltip in case the header content is cutoff.
    bodyContainer.title = bodyContainer.textContent || '';

    // If the container is empty, return null instead.
    return bodyContainer;
  }

  @computed get nonDebugLogs() {
    const logs = this.logs || [];

    return logs.filter((log) => !log.name.startsWith('$'));
  }

  @computed get debugLogs() {
    const logs = this.logs || [];

    return logs.filter((log) => log.name.startsWith('$'));
  }

  @computed get isPinned() {
    return this.userConfig?.build.steps.stepIsPinned(this.name);
  }

  @action setIsPinned(pinned: boolean) {
    this.userConfig?.build.steps.setStepPin(this.name, pinned);
  }

  @computed get isCritical() {
    return this.status !== BuildbucketStatus.Success || this.isPinned;
  }
  /**
   * true if and only if the step and all of its descendants succeeded.
   */
  @computed get succeededRecursively(): boolean {
    if (this.status !== BuildbucketStatus.Success) {
      return false;
    }
    return this.children.every((child) => child.succeededRecursively);
  }

  /**
   * true iff the step or one of its descendants failed (status Failure or InfraFailure).
   */
  @computed get failed(): boolean {
    if (
      this.status === BuildbucketStatus.Failure ||
      this.status === BuildbucketStatus.InfraFailure
    ) {
      return true;
    }
    return this.children.some((child) => child.failed);
  }

  @computed({ keepAlive: true }) get clusteredChildren() {
    return clusterBuildSteps(this.children);
  }

  /**
   * Returns instruction name for the step.
   * If the step does not have instruction name, return undefined.
   */
  @computed get instructionID(): string | undefined {
    const instructionTag = this.tags.find(
      (tag) => tag.key === 'resultdb.instruction.id',
    );
    return instructionTag?.value;
  }
}

/**
 * Split the steps into multiple groups such that each group maximally contains
 * consecutive steps that share the same criticality.
 *
 * Note that this function intentionally does not react to (i.e. untrack) the
 * steps' criticality, so that a change it the steps' criticality (e.g. when
 * the step is pinned/unpinned) does not trigger a rerun of the function.
 */
export function clusterBuildSteps(
  steps: readonly StepExt[],
): readonly (readonly StepExt[])[] {
  const clusters: StepExt[][] = [];
  for (const step of steps) {
    let lastCluster = clusters[clusters.length - 1];

    // Do not react to the change of a step's criticality because updating the
    // cluster base on the internal state of the step is confusing.
    // e.g. it can leads to the steps being re-clustered when users (un)pin a
    // step.
    const criticalityChanged = untracked(
      () => step.isCritical !== lastCluster?.[0]?.isCritical,
    );

    if (criticalityChanged) {
      lastCluster = [];
      clusters.push(lastCluster);
    }
    lastCluster.push(step);
  }
  return clusters;
}

export const BuildState = types
  .model('BuildState', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    data: types.frozen<Build>(),
    currentTime: types.safeReference(Timestamp),
    userConfig: types.safeReference(UserConfig),
  })
  .volatile(() => ({
    steps: [] as readonly StepExt[],
    rootSteps: [] as readonly StepExt[],
  }))
  .views((self) => ({
    get buildOrStepInfraFailed() {
      return (
        self.data.status === BuildbucketStatus.InfraFailure ||
        self.steps.some((s) => s.status === BuildbucketStatus.InfraFailure)
      );
    },
  }))
  .views((self) => {
    const clusteredRootSteps = keepAliveComputed(self, () =>
      clusterBuildSteps(self.rootSteps),
    );
    return {
      get clusteredRootSteps() {
        return clusteredRootSteps.get();
      },
    };
  })
  .actions((self) => ({
    afterCreate() {
      const steps: StepExt[] = [];
      const rootSteps: StepExt[] = [];

      // Map step name -> StepExt.
      const stepMap = new Map<string, StepExt>();

      // Build the step-tree.
      for (const step of self.data.steps || []) {
        const splitName = step.name.split('|');
        // There must be at least one element in a split string array.
        const selfName = splitName.pop()!;
        const depth = splitName.length;
        const parentName = splitName.join('|');
        const parent = stepMap.get(parentName);

        const index = (parent?.children || rootSteps).length;
        const listNumber = `${parent?.listNumber || ''}${index + 1}.`;
        const stepState = new StepExt({
          step,
          listNumber,
          depth,
          index,
          selfName,
          currentTime: self.currentTime,
          userConfig: self.userConfig,
        });

        steps.push(stepState);
        stepMap.set(step.name, stepState);
        if (!parent) {
          rootSteps.push(stepState);
        } else {
          parent.children.push(stepState);
        }
      }

      self.steps = steps;
      self.rootSteps = rootSteps;
    },
  }));

export type BuildStateInstance = Instance<typeof BuildState>;
export type BuildStateSnapshotIn = SnapshotIn<typeof BuildState>;
export type BuildStateSnapshotOut = SnapshotOut<typeof BuildState>;
