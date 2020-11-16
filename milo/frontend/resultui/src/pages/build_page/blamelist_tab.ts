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
import '../../components/hotkey';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { GitCommit } from '../../services/milo_internal';

export class BlamelistTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  @observable.ref private commits: GitCommit[] = [];
  @observable.ref private endOfPage = false;
  @observable.ref private precedingCommit!: GitCommit;
  @observable.ref private isLoading = true;

  @computed
  private get queryBlamelistResIter() {
    return this.buildState.queryBlamelistResIterFn();
  }

  @computed
  private get repoUrl() {
    const gitilesCommit = this.buildState.build!.input.gitilesCommit!;
    return `https://${gitilesCommit!.host}/${gitilesCommit.project}`;
  }

  @computed
  private get latestCommitId() {
    return this.buildState.build?.input.gitilesCommit?.id;
  }

  @computed
  private get revisionRange() {
      return this.endOfPage ? `${this.precedingCommit!.id.substring(0, 12)}..${this.latestCommitId!.substring(0, 12)}` : ``;
  }

  @computed
  private get blamelistSummary() {
      if (this.revisionRange) {
          return `This build included ${this.commits.length} new revisions from ${this.revisionRange}`;
      }
      if (this.commits.length > 0) {
          return `This build included over ${this.commits.length} new revisions up to ${this.latestCommitId!.substring(0, 12)}`;
      }
      return ``;
  }

  @computed
  private get gitilesLink() {
      if (this.revisionRange && this.repoUrl) {
          return `${this.repoUrl}/+log/${this.revisionRange}`;
      }
      return ``;
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
      this.precedingCommit = iter.value.precedingCommit;
      this.endOfPage = !iter.value.nextPageToken;
    }
    this.isLoading = false;
  }

  private allEntriesWereExpanded = true;
  private toggleAllEntries(expand: boolean) {
    this.allEntriesWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<CommitEntryElement>('milo-commit-entry')
      .forEach((e) => e.expanded = expand);
  }
  private readonly toggleAllEntriesByHotkey = () => this.toggleAllEntries(!this.allEntriesWereExpanded);

  protected render() {
    if (this.buildState.build && !this.buildState.build.input.gitilesCommit) {
      return html`
        <div id="no-blamelist">
          Blamelist is not available because the build has no associated gitiles commit.<br>
        </div>
      `;
    }

    return html`
      <div id="header">
        <div id="blamelist-summary">
          <span style=${styleMap({'display': this.blamelistSummary ? '' : 'none'})}>
            ${this.blamelistSummary}
          </span>
          <a href="${this.gitilesLink}" target="_blank" style=${styleMap({'display': this.gitilesLink ? '' : 'none'})}>
            [view in Gitiles]
          </a>
          <span id="load" style=${styleMap({'display': this.blamelistSummary && !this.endOfPage ? '' : 'none'})}>
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
        <milo-hotkey key="x" .handler=${this.toggleAllEntriesByHotkey} title="press x to expand/collapse all entries">
          <mwc-button
            class="action-button"
            dense unelevated
            @click=${() => this.toggleAllEntries(true)}
          >Expand All</mwc-button>
          <mwc-button
            class="action-button"
            dense unelevated
            @click=${() => this.toggleAllEntries(false)}
          >Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      <div id="main">
        ${this.commits.map((commit, i) => html`
        <milo-commit-entry
          .number=${i + 1}
          .repoUrl=${this.repoUrl}
          .commit=${commit}
          .expanded=${true}
        ></milo-commit-entry>
        `)}
        <div
          class="list-entry"
          style=${styleMap({'display': this.endOfPage && this.commits.length === 0 ? '' : 'none'})}
        >
          No blamelist.
        </div>
        <hr class="divider" style=${styleMap({'display': !this.endOfPage && this.commits.length === 0 ? 'none' : ''})}>
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
    #no-blamelist {
      padding: 10px;
    }

    #header {
      display: grid;
      grid-template-columns: 1fr auto;
      grid-gap: 5px;
      height: 28px;
      padding: 5px 10px;
    }

    #blamelist-summary {
      padding: 4px 0;
    }

    #main {
      padding-top: 5px;
      padding-left: 10px;
      border-top: 1px solid var(--divider-color);
    }
    milo-commit-entry {
      margin-bottom: 2px;
    }
    .list-entry {
      margin-top: 5px;
    }

    .divider {
      border: none;
      border-top: 1px solid var(--divider-color);
    }
    #load {
      color: var(--active-text-color);
    }
    #load-more {
      color: var(--active-text-color);
      cursor: pointer;
    }
  `;
}

customElement('milo-blamelist-tab')(
  consumeBuildState(
    consumeAppState(BlamelistTabElement),
  ),
);
