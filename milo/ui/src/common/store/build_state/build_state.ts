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
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { DateTime, Duration } from 'luxon';
import { action, computed, makeObservable, untracked } from 'mobx';
import { Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import {
  BLAMELIST_PIN_KEY,
  Build,
  BuildbucketStatus,
  getAssociatedGitilesCommit,
  Log,
  Step,
} from '@/common/services/buildbucket';
import { GitilesCommit, StringPair } from '@/common/services/common';
import { Timestamp, TimestampInstance } from '@/common/store/timestamp';
import { UserConfig, UserConfigInstance } from '@/common/store/user_config';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { parseProtoDurationStr } from '@/common/tools/time_utils';
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
   * summaryParts split summaryMarkdown into header and content.
   */
  // TODO(weiweilin): we should move this to build_step.ts because it contains
  // rendering logic.
  // TODO(weiweilin): this is a hack required to replicate the behavior of the
  // old build page. eventually, we probably want users to define headers
  // explicitly in another field.
  @computed get summaryParts() {
    const bodyContainer = document.createElement('div');
    render(
      unsafeHTML(renderMarkdown(this.summaryMarkdown || '')),
      bodyContainer,
    );
    // The body has no content.
    // We don't need to check bodyContainer.firstChild because text are
    // automatically wrapped in <p>.
    if (bodyContainer.firstElementChild === null) {
      return [null, null];
    }

    // We treat <div>s as paragraphs too because this is the behavior in the
    // legacy build page.
    const firstParagraph = bodyContainer.firstElementChild;
    if (!['P', 'DIV'].includes(firstParagraph.tagName)) {
      // The first element is not a paragraph, nothing is in the header.
      return [null, bodyContainer];
    }

    const headerContainer = document.createElement('span');

    // Finds all the nodes belongs to the header.
    while (firstParagraph.firstChild !== null) {
      // Found some text, move them from the body to the header.
      if (firstParagraph.firstChild !== firstParagraph.firstElementChild) {
        headerContainer.appendChild(
          firstParagraph.removeChild(firstParagraph.firstChild),
        );
        continue;
      }

      // Found an inline element, move it from the body to the header.
      if (
        ['A', 'SPAN', 'I', 'B', 'STRONG', 'CODE'].includes(
          firstParagraph.firstElementChild.tagName,
        )
      ) {
        headerContainer.appendChild(
          firstParagraph.removeChild(firstParagraph.firstElementChild),
        );
        continue;
      }

      // Found a line break, remove it from the body. The remaining nodes are
      // not in the header. Stop processing nodes.
      if (firstParagraph.firstElementChild.tagName === 'BR') {
        firstParagraph.removeChild(firstParagraph.firstElementChild);
        break;
      }

      // Found other (non-inline) elements. The remaining nodes are not in
      // the header. Stop processing nodes.
      break;
    }

    if (firstParagraph.firstChild === null) {
      bodyContainer.removeChild(firstParagraph);
    }

    // Show a tooltip in case the header content is cutoff.
    headerContainer.title = headerContainer.textContent || '';

    // If the container is empty, return null instead.
    return [
      headerContainer.firstChild ? headerContainer : null,
      bodyContainer.firstElementChild ? bodyContainer : null,
    ];
  }

  /**
   * Header of summaryMarkdown.
   *
   * It means to provide an overview of summaryMarkdown, and is shown in the UI
   * even when the corresponding step is collapsed.
   * Currently, it is the first line of the summaryMarkdown. For example, if
   * summaryMarkdown is 'header<br/>content' header should be 'header'.
   */
  @computed get header() {
    return this.summaryParts[0];
  }

  /**
   * Content of summaryMarkdown, aside from header.
   *
   * It is only shown in the UI if the corresponding step is expanded.
   * For example, if summaryMarkdown is 'header<br/>content' summary should be
   * 'content'.
   */
  @computed get summary() {
    return this.summaryParts[1];
  }

  @computed get filteredLogs() {
    const logs = this.logs || [];

    return this.userConfig?.build.steps.showDebugLogs ?? true
      ? logs
      : logs.filter((log) => !log.name.startsWith('$'));
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
    get createTime() {
      return DateTime.fromISO(self.data.createTime);
    },
    get startTime() {
      return self.data.startTime ? DateTime.fromISO(self.data.startTime) : null;
    },
    get endTime() {
      return self.data.endTime ? DateTime.fromISO(self.data.endTime) : null;
    },
    get cancelTime() {
      return self.data.cancelTime
        ? DateTime.fromISO(self.data.cancelTime)
        : null;
    },
    get schedulingTimeout() {
      return self.data.schedulingTimeout
        ? parseProtoDurationStr(self.data.schedulingTimeout)
        : null;
    },
    get executionTimeout() {
      return self.data.executionTimeout
        ? parseProtoDurationStr(self.data.executionTimeout)
        : null;
    },
    get gracePeriod() {
      return self.data.gracePeriod
        ? parseProtoDurationStr(self.data.gracePeriod)
        : null;
    },
    get buildOrStepInfraFailed() {
      return (
        self.data.status === BuildbucketStatus.InfraFailure ||
        self.steps.some((s) => s.status === BuildbucketStatus.InfraFailure)
      );
    },
    get buildNumOrId() {
      return self.data.number?.toString() || 'b' + self.data.id;
    },
    get isCanary() {
      return Boolean(
        self.data.input?.experiments?.includes(
          'luci.buildbucket.canary_software',
        ),
      );
    },
    get associatedGitilesCommit() {
      return getAssociatedGitilesCommit(self.data);
    },
    get blamelistPins(): readonly GitilesCommit[] {
      const blamelistPins =
        self.data.output?.properties?.[BLAMELIST_PIN_KEY] || [];
      if (blamelistPins.length === 0 && this.associatedGitilesCommit) {
        blamelistPins.push(this.associatedGitilesCommit);
      }
      return blamelistPins;
    },
    get recipeLink() {
      let csHost = 'source.chromium.org';
      if (self.data.exe?.cipdPackage?.includes('internal')) {
        csHost = 'source.corp.google.com';
      }
      // TODO(crbug.com/1149540): remove this conditional once the long-term
      // solution for recipe links has been implemented.
      if (self.data.builder.project === 'flutter') {
        csHost = 'cs.opensource.google';
      }
      const recipeName = self.data.input?.properties?.['recipe'];
      if (!recipeName) {
        return null;
      }

      return {
        label: recipeName as string,
        url: `https://${csHost}/search/?${new URLSearchParams([
          ['q', `file:recipes/${recipeName}.py`],
        ]).toString()}`,
        ariaLabel: `recipe ${recipeName}`,
      };
    },
    get _currentTime() {
      return self.currentTime?.dateTime || DateTime.now();
    },
    get pendingDuration() {
      return (this.startTime || this.endTime || this._currentTime).diff(
        this.createTime,
      );
    },
    get isPending() {
      return !this.startTime && !this.endTime;
    },
    /**
     * A build exceeded it's scheduling timeout when
     * - the build is canceled, AND
     * - the build did not enter the execution phase, AND
     * - the scheduling timeout is specified, AND
     * - the pending duration is no less than the scheduling timeout.
     */
    get exceededSchedulingTimeout() {
      return (
        !this.startTime &&
        !this.isPending &&
        this.schedulingTimeout !== null &&
        this.pendingDuration >= this.schedulingTimeout
      );
    },
    get executionDuration() {
      return this.startTime
        ? (this.endTime || this._currentTime).diff(this.startTime)
        : null;
    },
    get isExecuting() {
      return this.startTime !== null && !this.endTime;
    },
    /**
     * A build exceeded it's execution timeout when
     * - the build is canceled, AND
     * - the build had entered the execution phase, AND
     * - the execution timeout is specified, AND
     * - the execution duration is no less than the execution timeout.
     */
    get exceededExecutionTimeout(): boolean {
      return (
        self.data.status === BuildbucketStatus.Canceled &&
        this.executionDuration !== null &&
        this.executionTimeout !== null &&
        this.executionDuration >= this.executionTimeout
      );
    },
    get timeSinceCreated(): Duration {
      return this._currentTime.diff(this.createTime);
    },
    get timeSinceStarted(): Duration | null {
      return this.startTime ? this._currentTime.diff(this.startTime) : null;
    },
    get timeSinceEnded(): Duration | null {
      return this.endTime ? this._currentTime.diff(this.endTime) : null;
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
        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
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
