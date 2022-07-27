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

import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { DateTime } from 'luxon';
import MarkdownIt from 'markdown-it';
import { computed, makeObservable, observable } from 'mobx';

import './expandable_entry';
import { bugLine } from '../libs/markdown_it_plugins/bug_line';
import { bugnizerLink } from '../libs/markdown_it_plugins/bugnizer_link';
import { crbugLink } from '../libs/markdown_it_plugins/crbug_link';
import { defaultTarget } from '../libs/markdown_it_plugins/default_target';
import { reviewerLine } from '../libs/markdown_it_plugins/reviewer_line';
import { sanitizeHTML } from '../libs/sanitize_html';
import { NUMERIC_TIME_FORMAT } from '../libs/time_utils';
import { GitCommit } from '../services/milo_internal';
import commonStyle from '../styles/common_style.css';

const md = MarkdownIt('zero', { breaks: true, linkify: true })
  .enable(['linkify', 'newline'])
  .use(bugLine)
  .use(reviewerLine)
  .use(crbugLink)
  .use(bugnizerLink)
  .use(defaultTarget, '_blank');

/**
 * Renders an expandable entry of the given commit.
 */
@customElement('milo-commit-entry')
export class CommitEntryElement extends MobxLitElement {
  @observable.ref number = 0;
  @observable.ref repoUrl = '';
  @observable.ref commit!: GitCommit;
  onToggle = (_isExpanded: boolean) => {};

  @observable.ref private _expanded = false;
  get expanded() {
    return this._expanded;
  }
  set expanded(isExpanded) {
    if (isExpanded === this._expanded) {
      return;
    }
    this._expanded = isExpanded;
    this.onToggle(this._expanded);
  }

  @computed private get commitTime() {
    return DateTime.fromISO(this.commit.committer.time);
  }
  @computed private get commitTitle() {
    return this.commit.message.split('\n', 1)[0];
  }

  @computed private get descriptionHTML() {
    return sanitizeHTML(md.render(this.commit.message));
  }

  @computed private get changedFilenames() {
    return this.commit.treeDiff.map((diff) => {
      // If a file was moved, there is both an old and a new path, from which we
      // take only the new path.
      // If a file was deleted, its new path is /dev/null. In that case, we're
      // only interested in the old path.
      if (!diff.newPath || diff.newPath === '/dev/null') {
        return diff.oldPath;
      }
      return diff.newPath;
    });
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private renderChangedFiles() {
    return html`
      Changed files: <span class="greyed-out">${this.commit.treeDiff.length}</span>
      <ul>
        ${this.changedFilenames.map((filename) => html`<li>${filename}</li>`)}
      </ul>
    `;
  }

  protected render() {
    return html`
      <milo-expandable-entry .expanded=${this.expanded} .onToggle=${(expanded: boolean) => (this.expanded = expanded)}>
        <div slot="header" id="entry-header">
          <div id="header-number">${this.number}.</div>
          <div id="header-revision">
            <a href=${`${this.repoUrl}/+/${this.commit.id}`} target="_blank">${this.commit.id.substring(0, 12)}</a>
          </div>
          <div id="header-author">${this.commit.author.email}</div>
          <div id="header-time">${this.commitTime.toFormat(NUMERIC_TIME_FORMAT)}</div>
          <div id="header-description">${this.commitTitle}</div>
        </div>
        <div slot="content" id="entry-content">
          <div id="summary">${this.descriptionHTML}</div>
          ${this.renderChangedFiles()}
        </div>
      </milo-expandable-entry>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
      }

      #entry-header {
        display: flex;
      }

      #header-number {
        flex: 1;
        max-width: 30px;
        font-weight: bold;
      }

      #header-revision {
        flex: 1;
        max-width: 110px;
        font-weight: bold;
      }

      #header-author {
        flex: 1;
        max-width: 200px;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #header-time {
        flex: 1;
        max-width: 190px;
        padding-left: 10px;
      }

      #header-description {
        flex: 1;
        padding-left: 10px;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #entry-content {
        padding: 10px 20px 20px;
        border-bottom: 1px solid var(--divider-color);
      }

      #summary {
        margin-bottom: 15px;
      }
      #summary > p:first-child {
        margin-block-start: 0;
      }
      #summary > p:last-child {
        margin-block-end: 0;
      }

      .greyed-out {
        color: var(--greyed-out-text-color);
      }

      ul {
        margin: 3px 0;
        padding-inline-start: 28px;
      }
    `,
  ];
}
