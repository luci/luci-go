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

import { renderMarkdownUnsanitized } from '../libs/markdown_utils';
import { BuildStatus, Log, Step } from '../services/buildbucket';

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

  readonly selfName: string;
  readonly parentName: string | null;
  readonly children: StepExt[] = [];
  readonly renderTime: IObservableValue<DateTime>;

  constructor(step: Step, renderTime?: IObservableValue<DateTime>) {
    if (!step.name) {
      throw new Error('Step name can not be empty');
    }

    this.name = step.name;
    this.startTime = step.startTime ? DateTime.fromISO(step.startTime) : null;
    this.endTime = step.endTime ? DateTime.fromISO(step.endTime) : null;
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
   * canSplitSummaryHtmlCleanly checks if we can split summaryMarkdown into
   * header and content cleanly by the first <br/> tag.
   * For example:
   * 'header<br/>content' -> should return true
   * '<div>header<br/></div>content' should return false because after the
   * split, header would become '<div>header' and content would become
   * '</div>content'. They are invalid.
   */
  @computed private get canSplitSummaryHtmlCleanly(): boolean {
    // We enclose summaryMarkdown in div tag to prevent Markdown-it from adding
    // extra <p> tag to enclose everything.
    const markdown = `<div id="markdown_start">${this.summaryMarkdown || ''}</div>`;
    const summaryHtml = renderMarkdownUnsanitized(markdown);
    const domParser = new DOMParser();
    const parsed = domParser.parseFromString(summaryHtml, 'text/html');
    const brElements = parsed.querySelectorAll('br');
    // If parent of the first <br> element is not markdown_start, it means we cannot
    // break header and content cleanly.
    if (brElements.length > 0) {
      return brElements[0].parentElement?.getAttribute('id') === 'markdown_start';
    }
    return true;
  }

  /**
   * isHeaderValid checks if header contains invalid elements, in such cases it will
   * return false
   * Because header is shown in one line, it should not contain elements such as
   * <li>.
   */
  private isHeaderValid(header: string): boolean {
    const html = renderMarkdownUnsanitized(header);
    const domParser = new DOMParser();
    const parsed = domParser.parseFromString(html, 'text/html');
    // Do not allow header to contain <li> elements
    const eles = parsed.querySelectorAll('li');
    return eles.length === 0;
  }

  /**
   * summaryParts tries to split summaryMarkdown into header and content.
   * If it cannot be split cleanly, for example, if the result is invalid
   * html, or if the result header contains inappropriate elements, we
   * do not split and treat everything as content.
   */
  @computed get summaryParts() {
    if (!this.canSplitSummaryHtmlCleanly) {
      return ['', this.summaryMarkdown || ''];
    }

    const parts = this.summaryMarkdown?.split(/<br\/?>/i) || [''];
    const header = parts[0] || '';
    if (!this.isHeaderValid(header)) {
      return ['', this.summaryMarkdown || ''];
    }
    const content = parts.slice(1).join('<br>') || '';
    return [header, content];
  }

  /**
   * header of summaryMarkdown.
   * It means to provide an overview of summaryMarkdown, and is shown in the UI
   * even when the corresponding step is collapsed.
   * Currently, it is the first line of the summaryMarkdown, if the first line
   * can be extracted cleanly.
   * For example, if summaryMarkdown is 'header<br/>content'
   * header should be 'header'
   */
  @computed get header() {
    return this.summaryParts[0];
  }

  /**
   * content of summaryMarkdown, aside from header.
   * It is only shown in the UI if the corresponding step is expanded.
   * For example, if summaryMarkdown is 'header<br/>content'
   * summary should be 'content'
   */
  @computed get summary() {
    return this.summaryParts[1];
  }
}
