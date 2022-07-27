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

import { render } from 'lit-html';
import { DateTime, Duration } from 'luxon';
import { computed, IObservableValue, makeObservable, observable } from 'mobx';

import { renderMarkdown } from '../libs/markdown_utils';
import { BuildStatus, Log, Step } from '../services/buildbucket';

export interface StepInit {
  step: Step;
  depth: number;
  index: number;
  selfName: string;
  renderTime?: IObservableValue<DateTime>;
}

/**
 * Contains all fields of the Step object with added helper methods and
 * properties.
 */
export class StepExt {
  readonly name: string;
  readonly startTime: DateTime | null;
  readonly endTime: DateTime | null;
  readonly status: BuildStatus;
  readonly logs?: Log[] | undefined;
  readonly summaryMarkdown?: string | undefined;

  readonly depth: number;
  readonly index: number;
  readonly selfName: string;
  readonly children: StepExt[] = [];
  readonly renderTime: IObservableValue<DateTime>;

  constructor(init: StepInit) {
    makeObservable(this);

    const step = init.step;
    this.name = step.name;
    this.startTime = step.startTime ? DateTime.fromISO(step.startTime) : null;
    this.endTime = step.endTime ? DateTime.fromISO(step.endTime) : null;
    this.status = step.status;
    this.logs = step.logs;
    this.summaryMarkdown = step.summaryMarkdown;

    this.depth = init.depth;
    this.index = init.index;
    this.selfName = init.selfName;

    this.renderTime = init.renderTime || observable.box(DateTime.local());
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
    if (this.status === BuildStatus.Failure || this.status === BuildStatus.InfraFailure) {
      return true;
    }
    return this.children.some((child) => child.failed);
  }

  @computed get duration() {
    if (!this.startTime) {
      return Duration.fromMillis(0);
    }
    return (this.endTime || this.renderTime.get()).diff(this.startTime);
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

    render(renderMarkdown(this.summaryMarkdown || ''), bodyContainer);

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
}
