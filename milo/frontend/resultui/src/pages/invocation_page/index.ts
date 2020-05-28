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
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html, property } from 'lit-element';
import { autorun, computed, observable, when } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state_provider';
import { router } from '../../routes';
import { InvocationState } from '../../services/resultdb';
import { InvocationPageState, providePageState } from './context';
import './invocation_details_tab';

const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};

/**
 * Main test results page.
 * Reads invocation_id from URL params.
 * If not logged in, redirects to '/login?redirect=${current_url}'.
 * If invocation_id not provided, redirects to '/not-found'.
 * Otherwise, shows results for the invocation.
 */
export class InvocationPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @property() pageState = new InvocationPageState();

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const invocationId = location.params['invocation_id'];
    if (typeof invocationId !== 'string') {
      return cmd.redirect('/not-found');
    }
    this.pageState.invocationId = invocationId;
    return;
  }

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.disposers.push(autorun(
      () => this.pageState.appState = this.appState,
    ));
    this.disposers.push(when(
      () => this.pageState.invocationReq.state === 'rejected',
      () => Router.go(router.urlForName('error')),
    ));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  private renderInvocationState() {
    const invocation = this.pageState.invocation;
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
        href: router.urlForName(
          'test-results',
          {'invocation_id': this.pageState.invocationId},
        ),
      },
      {
        id: 'invocation-details',
        label: 'Invocation Details',
        href: router.urlForName(
          'invocation-details',
          {'invocation_id': this.pageState.invocationId},
        ),
      },
    ];
  }

  protected render() {
    if (this.pageState.invocationId === '') {
      return html``;
    }

    return html`
      <div id="test-invocation-summary">
        <div id="test-invocation-id">
          <span id="test-invocation-id-label">Invocation ID </span>
          <span>${this.pageState.invocationId}</span>
        </div>
        <div id="test-invocation-state">${this.renderInvocationState()}</div>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.pageState.invocationReq.state === 'pending'}
      ></tr-status-bar>
      <tr-tab-bar
        .tabs=${this.tabDefs}
        .selectedTabId=${this.pageState.selectedTabId}
      ></tr-tab-bar>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      height: calc(100vh - var(--header-height));
      display: grid;
      grid-template-rows: repeat(3, auto) 1fr;
    }

    #test-invocation-summary {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
      display: flex;
    }

    #test-invocation-id {
      flex: 0 auto;
    }

    #test-invocation-id-label {
      color: rgb(95, 99, 104);
    }

    #test-invocation-state {
      margin-left: auto;
      flex: 0 auto;
    }
  `;
}

customElement('tr-invocation-page')(
  providePageState(
    consumeAppState(
      InvocationPageElement,
    ),
  ),
);
