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

import { MobxLitElement } from '@adobe/lit-mobx';
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { DateTime } from 'luxon';
import MarkdownIt from 'markdown-it';
import { computed, observable } from 'mobx';
import { bugnizerLink } from '../libs/markdown_it_plugins/bugnizer_link';

import { bugLine } from '../libs/markdown_it_plugins/bug_line';
import { crbugLink } from '../libs/markdown_it_plugins/crbug_link';
import { defaultTarget } from '../libs/markdown_it_plugins/default_target';
import { reviewerLine } from '../libs/markdown_it_plugins/reviewer_line';
import { sanitizeHTML } from '../libs/sanitize_html';
import { DEFAULT_TIME_FORMAT } from '../libs/time_utils';
import { GitCommit } from '../services/milo_internal';
import './expandable_entry';

const md = MarkdownIt('zero', {breaks: true, linkify: true})
  .enable(['linkify', 'newline'])
  .use(bugLine)
  .use(reviewerLine)
  .use(crbugLink)
  .use(bugnizerLink)
  .use(defaultTarget, '_blank');

/**
 * Renders an expandable entry of the given commit.
 */
export class CommitEntryElement extends MobxLitElement {
  @observable.ref number = 0;
  @observable.ref repoUrl = '';
  @observable.ref commit!: GitCommit;
  onToggle = (_isExpanded: boolean) => {};

  @observable.ref private _expanded = false;
  get expanded() { return this._expanded; }
  set expanded(isExpanded) {
    if (isExpanded === this._expanded) {
      return;
    }
    this._expanded = isExpanded;
    this.onToggle(this._expanded);
  }

  @computed get commitTime() { return DateTime.fromISO(this.commit.committer.time); }
  @computed get title() { return this.commit.message.split('\n', 1)[0]; }
  @computed get descriptionLines() {
    const lines = this.commit.message.split('\n');
    lines.shift();
    if (lines[0].length === 0) {
      lines.shift();
    }
    return lines;
  }

  @computed get description() {
    return this.commit.message.slice(this.title.length + 1);
  }

  @computed get descriptionHTML() {
    return sanitizeHTML(md.render(this.description));
  }

  protected render() {
    return html`
      <milo-expandable-entry
        .expanded=${this.expanded}
        .onToggle=${(expanded: boolean) => this.expanded = expanded}
      >
        <span slot="header">
          <b>${this.number}. ${this.title}</b> <i>by ${this.commit.author.name} at ${this.commitTime.toFormat(DEFAULT_TIME_FORMAT)}</i>
        </span>
        <div slot="content">
          <table slot="content" border="0">
            <tr><td>Changed by:</td><td>${this.commit.author.name} - ${this.commit.author.email}</td></tr>
            <tr><td>Changed at:</td><td>${this.commitTime.toFormat(DEFAULT_TIME_FORMAT)}</td></tr>
            <tr><td>Revision:</td><td><a href=${`${this.repoUrl}/+/${this.commit.id}`} target="_blank">${this.commit.id}</a></td></tr>
          </table>
          <div id="summary">${this.descriptionHTML}</div>
        </div>
      </milo-expandable-entry>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    #summary {
      background-color: var(--block-background-color);
      padding: 5px;
    }
    #summary > p:first-child {
      margin-block-start: 0;
    }
    #summary > p:last-child {
      margin-block-end: 0;
    }
  `;
}

customElement('milo-commit-entry')(CommitEntryElement);
