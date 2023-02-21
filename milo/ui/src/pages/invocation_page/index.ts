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
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import '../../components/timeline';
import '../test_results_tab/count_indicator';
import { MiloBaseElement } from '../../components/milo_base';
import { TabDef } from '../../components/tab_bar';
import { INVOCATION_STATE_DISPLAY_MAP } from '../../libs/constants';
import { consumer, provider } from '../../libs/context';
import { reportRenderError } from '../../libs/error_handler';
import { getBuildURLPathFromBuildId, getInvURLPath } from '../../libs/url_utils';
import { consumeStore, StoreInstance } from '../../store';
import { provideInvocationState } from '../../store/invocation_state';
import commonStyle from '../../styles/common_style.css';
import { provideProject } from '../test_results_tab/test_variants_table/context';

/**
 * Main test results page.
 * Reads invocation_id from URL params.
 * If not logged in, redirects to '/login?redirect=${current_url}'.
 * If invocation_id not provided, redirects to '/not-found'.
 * Otherwise, shows results for the invocation.
 */
@customElement('milo-invocation-page')
@provider
@consumer
export class InvocationPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @provideInvocationState()
  @computed
  get invState() {
    return this.store.invocationPage.invocation;
  }

  @provideProject({ global: true })
  @computed
  get project() {
    return this.store.invocationPage.invocation.project ?? undefined;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private invocationId = '';
  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const invocationId = location.params['invocation_id'];
    if (typeof invocationId !== 'string') {
      return cmd.redirect('/ui/not-found');
    }
    this.invocationId = invocationId;
    return;
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => [this.store.invocationPage],
        ([pageState]) => {
          pageState.setInvocationId(this.invocationId);
        },
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => [this.invState],
        ([invState]) => {
          // Emulate @property() update.
          this.updated(new Map([['invState', invState]]));
        },
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => this.project,
        (project) => {
          // Emulate @property() update.
          this.updated(new Map([['project', project]]));
        },
        { fireImmediately: true }
      )
    );

    document.title = `inv: ${this.invocationId}`;
  }

  private renderInvocationState() {
    const invocation = this.invState.invocation;
    if (!invocation) {
      return null;
    }
    if (invocation.finalizeTime) {
      return html`
        <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
        at ${new Date(invocation.finalizeTime).toLocaleString()}
      `;
    }

    return html`
      <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
      since ${new Date(invocation.createTime).toLocaleString()}
    `;
  }

  @computed get tabDefs(): TabDef[] {
    return [
      {
        id: 'test-results',
        label: 'Test Results',
        href: `${getInvURLPath(this.invState.invocationId!)}/test-results`,
        slotName: 'test-count-indicator',
      },
      {
        id: 'invocation-details',
        label: 'Invocation Details',
        href: `${getInvURLPath(this.invState.invocationId!)}/invocation-details`,
      },
    ];
  }

  protected render = reportRenderError(this, () => {
    if (this.invState.invocationId === '') {
      return html``;
    }

    return html`
      <div id="test-invocation-summary">
        <div id="test-invocation-id">
          <span id="test-invocation-id-label">Invocation ID </span>
          <span>${this.invState.invocationId}</span>
          ${this.renderBuildLink(this.invState.invocationId)} ${this.renderTaskLink(this.invState.invocationId)}
        </div>
        <div id="test-invocation-state">${this.renderInvocationState()}</div>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
        .loading=${this.invState.invocation === null}
      ></milo-status-bar>
      <milo-tab-bar .tabs=${this.tabDefs} .selectedTabId=${this.store.selectedTabId}>
        <milo-trt-count-indicator slot="test-count-indicator"></milo-trt-count-indicator>
      </milo-tab-bar>
      <slot></slot>
    `;
  });

  private renderBuildLink(invId: string | null) {
    if (!invId) {
      return '';
    }
    const match = invId.match(/^build-(?<id>\d+)/);
    if (!match) {
      return '';
    }

    return html`(<a href=${getBuildURLPathFromBuildId(match.groups!['id'])} target="_blank">build page</a>)`;
  }

  // Should be checked upstream, but allowlist URLs here just to be safe.
  private static allowedSwarmingUrls = [
    'chromium-swarm-dev.appspot.com',
    'chromium-swarm.appspot.com',
    'chrome-swarming.appspot.com',
  ];

  private renderTaskLink(invId: string | null) {
    if (!invId) {
      return '';
    }
    const match = invId.match(/^task-(?<url>.*)-(?<id>[0-9a-fA-F]+)$/);
    if (!match) {
      return '';
    }
    const url = match.groups!['url'];
    if (InvocationPageElement.allowedSwarmingUrls.indexOf(url) === -1) {
      return '(unknown swarming url)';
    }
    const taskPageUrl = `https://${url}/task?id=${match.groups!['id']}`;
    return html`(<a href=${taskPageUrl} target="_blank">task page</a>)`;
  }

  static styles = [
    commonStyle,
    css`
      #test-invocation-summary {
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
        display: flex;
      }

      milo-tab-bar {
        margin: 0 10px;
        padding-top: 10px;
      }

      milo-trt-count-indicator {
        margin-right: -13px;
      }

      #test-invocation-id {
        flex: 0 auto;
      }

      #test-invocation-id-label {
        color: var(--light-text-color);
      }

      #test-invocation-state {
        margin-left: auto;
        flex: 0 auto;
      }
    `,
  ];
}
