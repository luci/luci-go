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

import '@material/mwc-button';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable, reaction } from 'mobx';

import '../../components/commit_entry';
import '../../components/dot_spinner';
import '../../components/hotkey';
import { CommitEntryElement } from '../../components/commit_entry';
import { MiloBaseElement } from '../../components/milo_base';
import {
  GA_ACTIONS,
  GA_CATEGORIES,
  trackEvent,
} from '../../libs/analytics_utils';
import { consumer } from '../../libs/context';
import {
  errorHandler,
  forwardWithoutMsg,
  reportErrorAsync,
  reportRenderError,
} from '../../libs/error_handler';
import { getGitilesRepoURL } from '../../libs/url_utils';
import { GitCommit } from '../../services/milo_internal';
import { consumeStore, StoreInstance } from '../../store';
import { commonStyles } from '../../styles/stylesheets';

@customElement('milo-blamelist-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class BlamelistTabElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref private commits: GitCommit[] = [];
  @observable.ref private endOfPage = false;
  @observable.ref private precedingCommit?: GitCommit;
  @observable.ref private isLoading = true;

  @computed
  private get queryBlamelistResIter() {
    const iterFn =
      this.store.buildPage.queryBlamelistResIterFns?.[
        this.store.buildPage.selectedBlamelistPinIndex
      ] ||
      async function* () {
        yield Promise.race([]);
      };

    return iterFn();
  }

  @computed
  get selectedRepoURL() {
    if (!this.selectedBlamelistPin) {
      return null;
    }
    return getGitilesRepoURL(this.selectedBlamelistPin);
  }

  @computed
  get selectedBlamelistPin() {
    return this.store.buildPage.build?.blamelistPins[
      this.store.buildPage.selectedBlamelistPinIndex
    ];
  }

  @computed
  private get revisionRange() {
    const blamelistPin = this.selectedBlamelistPin;
    if (!this.endOfPage || !blamelistPin) {
      return '';
    }

    return this.precedingCommit
      ? `${this.precedingCommit.id.substring(0, 12)}..${
          blamelistPin.id?.substring(0, 12) || blamelistPin.ref
        }`
      : blamelistPin.id?.substring(0, 12) || blamelistPin.ref;
  }

  @computed
  private get blamelistSummary() {
    if (this.revisionRange) {
      return `This build included ${this.commits.length} new revisions from ${this.revisionRange}`;
    }
    if (this.commits.length > 0) {
      return `This build included over ${
        this.commits.length
      } new revisions up to ${
        this.selectedBlamelistPin!.id?.substring(0, 12) ||
        this.selectedBlamelistPin!.ref
      }`;
    }
    return '';
  }

  @computed
  private get gitilesLink() {
    if (this.revisionRange && this.selectedRepoURL) {
      return `${this.selectedRepoURL}/+log/${this.revisionRange}`;
    }
    return '';
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.store.setSelectedTabId('blamelist');
    trackEvent(
      GA_CATEGORIES.BLAMELIST_TAB,
      GA_ACTIONS.TAB_VISITED,
      window.location.href
    );
    this.addDisposer(
      reaction(
        () => this.queryBlamelistResIter,
        () => {
          this.commits = [];
          this.loadNextPage();
        },
        { fireImmediately: true }
      )
    );
  }

  private loadNextPage = reportErrorAsync(this, async () => {
    this.isLoading = true;
    this.endOfPage = false;
    const iter = await this.queryBlamelistResIter.next();
    if (iter.done) {
      this.endOfPage = true;
    } else {
      this.commits = this.commits.concat(iter.value.commits);
      this.precedingCommit = iter.value.precedingCommit;
      this.endOfPage = !iter.value.nextPageToken;
    }
    this.isLoading = false;
  });

  private allEntriesWereExpanded = false;
  private toggleAllEntries(expand: boolean) {
    this.allEntriesWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<CommitEntryElement>(
      'milo-commit-entry'
    ).forEach((e) => (e.expanded = expand));
  }
  private readonly toggleAllEntriesByHotkey = () =>
    this.toggleAllEntries(!this.allEntriesWereExpanded);

  protected render = reportRenderError(this, () => {
    if (this.store.buildPage.build && !this.selectedBlamelistPin) {
      return html`
        <div id="no-blamelist">
          Blamelist is not available because the build has no associated gitiles
          commit.<br />
        </div>
      `;
    }

    return html`
      <div id="header">
        <div id="repo-selector">
          <label for="repo-select">Repo:</label>
          <select
            id="repo-select"
            @input=${(e: InputEvent) =>
              this.store.buildPage.setSelectedBlamelist(
                Number((e.target as HTMLOptionElement).value)
              )}
          >
            ${this.store.buildPage.build?.blamelistPins.map(
              (pin, i) => html`
                <option
                  value=${i}
                  ?selected=${this.store.buildPage.selectedBlamelistPinIndex ===
                  i}
                >
                  ${getGitilesRepoURL(pin)}
                </option>
              `
            )}
          </select>
        </div>
        <milo-hotkey
          .key=${'x'}
          .handler=${this.toggleAllEntriesByHotkey}
          title="press x to expand/collapse all entries"
        >
          <mwc-button
            class="action-button"
            dense
            unelevated
            @click=${() => this.toggleAllEntries(true)}
          >
            Expand All
          </mwc-button>
          <mwc-button
            class="action-button"
            dense
            unelevated
            @click=${() => this.toggleAllEntries(false)}
          >
            Collapse All
          </mwc-button>
        </milo-hotkey>
      </div>
      <div id="main">
        <div
          id="blamelist-summary"
          class="list-entry"
          style=${styleMap({ display: this.blamelistSummary ? '' : 'none' })}
        >
          <span>${this.blamelistSummary}</span>
          <a
            href="${this.gitilesLink}"
            target="_blank"
            style=${styleMap({ display: this.gitilesLink ? '' : 'none' })}
          >
            [view in Gitiles]
          </a>
          <span
            id="load"
            style=${styleMap({
              display: this.blamelistSummary && !this.endOfPage ? '' : 'none',
            })}
          >
            <span
              id="load-more"
              style=${styleMap({ display: this.isLoading ? 'none' : '' })}
              @click=${this.loadNextPage}
            >
              Load More
            </span>
            <span style=${styleMap({ display: this.isLoading ? '' : 'none' })}>
              Loading <milo-dot-spinner></milo-dot-spinner>
            </span>
          </span>
        </div>
        <hr
          class="divider"
          style=${styleMap({ display: this.blamelistSummary ? '' : 'none' })}
        />
        ${this.commits.map(
          (commit, i) => html`
            <milo-commit-entry
              .number=${i + 1}
              .repoUrl=${this.selectedRepoURL}
              .commit=${commit}
              .expanded=${this.commits.length === 1}
            ></milo-commit-entry>
          `
        )}
        <div
          class="list-entry"
          style=${styleMap({
            display: this.endOfPage && this.commits.length === 0 ? '' : 'none',
          })}
        >
          No blamelist.
        </div>
        <hr
          class="divider"
          style=${styleMap({
            display: !this.endOfPage && this.commits.length === 0 ? 'none' : '',
          })}
        />
        <div class="list-entry">
          <span>Showing ${this.commits.length} commits.</span>
          <span
            id="load"
            style=${styleMap({ display: this.endOfPage ? 'none' : '' })}
          >
            <span
              id="load-more"
              style=${styleMap({ display: this.isLoading ? 'none' : '' })}
              @click=${this.loadNextPage}
            >
              Load More
            </span>
            <span style=${styleMap({ display: this.isLoading ? '' : 'none' })}>
              Loading <milo-dot-spinner></milo-dot-spinner>
            </span>
          </span>
        </div>
      </div>
    `;
  });

  static styles = [
    commonStyles,
    css`
      #no-blamelist {
        padding: 10px;
      }

      #header {
        display: grid;
        grid-template-columns: 1fr auto;
        grid-gap: 5px;
        height: 30px;
        padding: 5px 10px 3px 10px;
      }

      mwc-button {
        margin-top: 1px;
        width: var(--expand-button-width);
      }

      #repo-selector {
        white-space: nowrap;
        padding-left: 5px;
      }
      #repo-select {
        display: inline-block;
        width: 450px;
        padding: 0.27rem 0.5rem;
        font-size: 1rem;
        color: var(--light-text-color);
        background-clip: padding-box;
        border: 1px solid var(--divider-color);
        border-radius: 0.25rem;
        transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
        text-overflow: ellipsis;
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
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-blamelist-tab': Record<string, never>;
    }
  }
}

export function BlamelistTab() {
  return <milo-blamelist-tab />;
}
