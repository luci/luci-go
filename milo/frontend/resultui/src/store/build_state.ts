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

import { render } from 'lit-html';
import { DateTime, Duration } from 'luxon';
import { IAnyModelType, IMSTArray, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { renderMarkdown } from '../libs/markdown_utils';
import { parseProtoDuration } from '../libs/time_utils';
import { BLAMELIST_PIN_KEY, Build, BuildStatus, GitilesCommit, Step } from '../services/buildbucket';
import { Timestamp } from './timestamp';

export interface StepExt extends Step {
  /**
   * The number assigned to the step in a nested list. For example, if a step is
   * the 1st child of the 2nd root step, the list number would be '2.1.'.
   */
  listNumber: string;
  depth: number;
  index: number;
  selfName: string;
}

export const BuildStepState = types
  .model('BuildStepState', {
    id: types.identifier,
    data: types.frozen<StepExt>(),
    currentTime: types.safeReference(Timestamp),

    _children: types.array(types.late((): IAnyModelType => BuildStepState)),
  })
  .views((self) => ({
    get children() {
      return self._children as IMSTArray<typeof BuildStepState>;
    },
    /**
     * true iff the step and all of its descendants succeeded.
     */
    get succeededRecursively(): boolean {
      if (self.data.status !== BuildStatus.Success) {
        return false;
      }
      return this.children.every((child) => child.succeededRecursively);
    },
    /**
     * true iff the step or one of its descendants failed (status Failure or
     * InfraFailure).
     */
    get failed(): boolean {
      if (self.data.status === BuildStatus.Failure || self.data.status === BuildStatus.InfraFailure) {
        return true;
      }
      return this.children.some((child) => child.failed);
    },
    get _currentTime() {
      return self.currentTime?.dateTime || DateTime.now();
    },
    get startTime() {
      return self.data.startTime ? DateTime.fromISO(self.data.startTime) : null;
    },
    get endTime() {
      return self.data.endTime ? DateTime.fromISO(self.data.endTime) : null;
    },
    get duration() {
      if (!this.startTime) {
        return Duration.fromMillis(0);
      }
      return (this.endTime || this._currentTime).diff(this.startTime);
    },
    /**
     * summaryParts split summaryMarkdown into header and content.
     */
    // TODO(weiweilin): we should move this to build_step.ts because it contains
    // rendering logic.
    // TODO(weiweilin): this is a hack required to replicate the behavior of the
    // old build page. eventually, we probably want users to define headers
    // explicitly in another field.
    get summaryParts() {
      const bodyContainer = document.createElement('div');

      render(renderMarkdown(self.data.summaryMarkdown || ''), bodyContainer);

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
          headerContainer.appendChild(firstParagraph.removeChild(firstParagraph.firstChild));
          continue;
        }

        // Found an inline element, move it from the body to the header.
        if (['A', 'SPAN', 'I', 'B', 'STRONG', 'CODE'].includes(firstParagraph.firstElementChild.tagName)) {
          headerContainer.appendChild(firstParagraph.removeChild(firstParagraph.firstElementChild));
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
    },
    /**
     * Header of summaryMarkdown.
     *
     * It means to provide an overview of summaryMarkdown, and is shown in the UI
     * even when the corresponding step is collapsed.
     * Currently, it is the first line of the summaryMarkdown. For example, if
     * summaryMarkdown is 'header<br/>content' header should be 'header'.
     */
    get header() {
      return this.summaryParts[0];
    },
    /**
     * Content of summaryMarkdown, aside from header.
     *
     * It is only shown in the UI if the corresponding step is expanded.
     * For example, if summaryMarkdown is 'header<br/>content' summary should be
     * 'content'.
     */
    get summary() {
      return this.summaryParts[1];
    },
  }));

export type BuildStepStateInstance = Instance<typeof BuildStepState>;
export type BuildStepStateSnapshotIn = SnapshotIn<typeof BuildStepState>;
export type BuildStepStateSnapshotOut = SnapshotOut<typeof BuildStepState>;

export const BuildState = types
  .model('BuildState', {
    data: types.frozen<Build>(),
    currentTime: types.safeReference(Timestamp),

    _steps: types.array(BuildStepState),
    _rootSteps: types.array(types.reference(BuildStepState)),
  })
  .views((self) => ({
    get steps() {
      return self._steps;
    },
    get rootSteps() {
      return self._rootSteps;
    },
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
      return self.data.cancelTime ? DateTime.fromISO(self.data.cancelTime) : null;
    },
    get schedulingTimeout() {
      return self.data.schedulingTimeout ? Duration.fromMillis(parseProtoDuration(self.data.schedulingTimeout)) : null;
    },
    get executionTimeout() {
      return self.data.executionTimeout ? Duration.fromMillis(parseProtoDuration(self.data.executionTimeout)) : null;
    },
    get gracePeriod() {
      return self.data.gracePeriod ? Duration.fromMillis(parseProtoDuration(self.data.gracePeriod)) : null;
    },
    get buildOrStepInfraFailed() {
      return (
        self.data.status === BuildStatus.InfraFailure ||
        this.steps.some((s) => s.data.status === BuildStatus.InfraFailure)
      );
    },
    get buildNumOrId() {
      return self.data.number?.toString() || 'b' + self.data.id;
    },
    get isCanary() {
      return Boolean(self.data.input?.experiments?.includes('luci.buildbucket.canary_software'));
    },
    get buildSets(): readonly string[] {
      return self.data.tags.filter((tag) => tag.key === 'buildset').map((tag) => tag.value);
    },
    get associatedGitilesCommit() {
      return self.data.output?.gitilesCommit || self.data.input?.gitilesCommit;
    },
    get blamelistPins(): readonly GitilesCommit[] {
      const blamelistPins = self.data.output?.properties?.[BLAMELIST_PIN_KEY] || [];
      if (blamelistPins.length === 0 && this.associatedGitilesCommit) {
        blamelistPins.push(this.associatedGitilesCommit);
      }
      return blamelistPins;
    },
    get recipeLink() {
      let csHost = 'source.chromium.org';
      if (self.data.exe.cipdPackage.includes('internal')) {
        csHost = 'source.corp.google.com';
      }
      // TODO(crbug.com/1149540): remove this conditional once the long-term
      // solution for recipe links has been implemented.
      if (self.data.builder.project === 'flutter') {
        csHost = 'cs.opensource.google';
      }
      const recipeName = self.data.input?.properties?.['recipe'] as string;
      return {
        label: recipeName,
        url: `https://${csHost}/search/?${new URLSearchParams([['q', `file:recipes/${recipeName}.py`]]).toString()}`,
        ariaLabel: `recipe ${recipeName}`,
      };
    },
    get _currentTime() {
      return self.currentTime?.dateTime || DateTime.now();
    },
    get pendingDuration() {
      return (this.startTime || this.endTime || this._currentTime).diff(this.createTime);
    },
    get isPending() {
      return !this.startTime && !this.endTime;
    },
    /**
     * A build exceeded it's scheduling timeout when
     * * the build is canceled, AND
     * * the build did not enter the execution phase, AND
     * * the scheduling timeout is specified, AND
     * * the pending duration is no less than the scheduling timeout.
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
      return this.startTime ? (this.endTime || this._currentTime).diff(this.startTime) : null;
    },
    get isExecuting() {
      return this.startTime !== null && !this.endTime;
    },
    /**
     * A build exceeded it's execution timeout when
     * * the build is canceled, AND
     * * the build had entered the execution phase, AND
     * * the execution timeout is specified, AND
     * * the execution duration is no less than the execution timeout.
     */
    get exceededExecutionTimeout(): boolean {
      return (
        self.data.status === BuildStatus.Canceled &&
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
  .actions((self) => ({
    afterCreate() {
      const stepStates: BuildStepStateSnapshotIn[] = [];
      const rootStepIds: string[] = [];

      // Map step name -> BuildStepStateSnapshotIn.
      const stepMap = new Map<string, BuildStepStateSnapshotIn>();

      // Build the step-tree.
      for (const step of self.data.steps || []) {
        const splitName = step.name.split('|');
        const selfName = splitName.pop()!;
        const depth = splitName.length;
        const parentName = splitName.join('|');
        const parent = stepMap.get(parentName);

        const index = (parent?._children || rootStepIds).length;
        const listNumber = `${parent?.data.listNumber || ''}${index + 1}.`;
        const stepState = {
          id: step.name,
          data: { ...step, listNumber, depth, index, selfName },
          currentTime: self.currentTime?.id,
          _children: [],
        };

        stepStates.push(stepState);
        stepMap.set(step.name, stepState);
        if (!parent) {
          rootStepIds.push(selfName);
        } else {
          parent._children!.push(stepState);
        }
      }

      self._steps.clear();
      self._rootSteps.clear();
      self._steps.push(...stepStates);
      self._rootSteps.push(...rootStepIds);
    },
  }));

export type BuildStateInstance = Instance<typeof BuildState>;
export type BuildStateSnapshotIn = SnapshotIn<typeof BuildState>;
export type BuildStateSnapshotOut = SnapshotOut<typeof BuildState>;
