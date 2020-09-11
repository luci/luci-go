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
import '@material/mwc-button';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../../components/commit_entry';
import { CommitEntryElement } from '../../components/commit_entry';
import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { GitCommit } from '../../services/milo_internal';

export class BlamelistTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  @observable.ref private commits: GitCommit[] = [];
  @observable.ref private endOfPage = false;
  @observable.ref private isLoading = true;
  private get queryBlamelistResIter() { return this.buildState.queryBlamelistResIterFn(); }

  @computed
  private get repoUrl() {
    const gitilesCommit = this.buildState.buildPageData!.input.gitiles_commit!;
    return `https://${gitilesCommit!.host}/${gitilesCommit.project}`;
  }

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'blamelist';
    this.disposer = reaction(
      () => this.queryBlamelistResIter,
      () => {
        this.commits = [];
        this.loadNextPage();
      },
      {fireImmediately: true},
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  private loadNextPage = async () => {
    this.isLoading = true;
    const iter = await this.queryBlamelistResIter.next();
    if (iter.done) {
      this.endOfPage = true;
    } else {
      this.commits = this.commits.concat(iter.value.commits);
      this.endOfPage = !iter.value.next_page_token;
    }
    this.isLoading = false;
  }

  private toggleAllVariants(expand: boolean) {
    this.shadowRoot!.querySelectorAll<CommitEntryElement>('milo-commit-entry')
      .forEach((e) => e.expanded = expand);
  }

  protected render() {
    return html`
      <div id="header">
        <span></span>
        <mwc-button
          class="action-button"
          dense unelevated
          @click=${() => this.toggleAllVariants(true)}
        >Expand All</mwc-button>
        <mwc-button
          class="action-button"
          dense unelevated
          @click=${() => this.toggleAllVariants(false)}
        >Collapse All</mwc-button>
      </div>
      <div id="main">
        ${this.commits.map((commit, i) => html`
        <milo-commit-entry
          .number=${i + 1}
          .repoUrl=${this.repoUrl}
          .commit=${commit}
        ></milo-commit-entry>
        `)}
        <hr class="divider" style=${styleMap({'display': this.commits.length === 0 ? 'none' : ''})}>
        <div class="list-entry">
          <span>Showing ${this.commits.length} commits.</span>
          <span id="load" style=${styleMap({'display': this.endOfPage ? 'none' : ''})}>
            <span
              id="load-more"
              style=${styleMap({'display': this.isLoading ? 'none' : ''})}
              @click=${this.loadNextPage}
            >
              Load More
            </span>
            <span style=${styleMap({'display': this.isLoading ? '' : 'none'})}>
              Loading <milo-dot-spinner></milo-dot-spinner>
            </span>
          </span>
        </div>
      </div>
    `;
  }

  static styles = css`
    #header {
      display: grid;
      grid-template-columns: 1fr auto auto;
      grid-gap: 5px;
      height: 28px;
      padding: 5px 10px 3px 10px;
    }

    .action-button {
      --mdc-theme-primary: rgb(0, 123, 255);
    }

    #main {
      padding-top: 5px;
      padding-left: 10px;
      border-top: 1px solid #DDDDDD;
    }
    milo-commit-entry {
      margin-bottom: 2px;
    }
    .list-entry {
      margin-top: 5px;
    }

    .divider {
      border: none;
      border-top: 1px solid rgb(235, 235, 235);
    }
    #load {
      color: blue;
    }
    #load-more {
      color: blue;
      cursor: pointer;
    }
  `;
}

customElement('milo-blamelist-tab')(
  consumeBuildState(
    consumeAppState(BlamelistTabElement),
  ),
);
