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
import '@material/mwc-dialog';
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import merge from 'lodash-es/merge';
import { observable, reaction } from 'mobx';

import '../components/signin';
import { UserUpdateEvent } from '../components/signin';
import { AppState, provideAppState } from '../context/app_state/app_state';
import { DEFAULT_USER_CONFIGS, provideConfigsStore, UserConfigs, UserConfigsStore } from '../context/app_state/user_configs';

const gAuthPromise = new Promise<gapi.auth2.GoogleAuth>((resolve, reject) => {
  window.gapi?.load('auth2', () => {
    gapi.auth2
      .init({client_id: CONFIGS.OAUTH2.CLIENT_ID, scope: 'email'})
      .then(resolve, reject);
  });
});

function genFeedbackUrl() {
  const feedbackComment = encodeURIComponent(
`From Link: ${document.location.href}
Please enter a description of the problem, with repro steps if applicable.
`);
  return `https://bugs.chromium.org/p/chromium/issues/entry?template=Build%20Infrastructure&components=Infra%3EPlatform%3EMilo%3EResultUI&labels=Pri-2,Type-Bug&comment=${feedbackComment}`;
}

// An array of [buildTabName, buildTabLabel] tuples.
// Use an array of tuples instead of an Object to ensure order.
const TAB_NAME_LABEL_TUPLES = Object.freeze([
  Object.freeze(['build-overview', 'Overview']),
  Object.freeze(['build-steps', 'Steps & Logs']),
  Object.freeze(['build-related-builds', 'Related Builds']),
  Object.freeze(['build-test-results', 'Test Results']),
  Object.freeze(['build-timeline', 'Timeline']),
  Object.freeze(['build-blamelist', 'Blamelist']),
]);

/**
 * Renders page header, including a sign-in widget, a settings button, and a
 * feedback button, at the top of the child nodes.
 * Refreshes the page when a new clientId is provided.
 */
@customElement('milo-page-layout')
@provideConfigsStore
@provideAppState
export class PageLayoutElement extends MobxLitElement {
  readonly appState = new AppState();
  readonly configsStore = new UserConfigsStore();

  @observable.ref private showSettingsDialog = false;

  @observable private readonly uncommittedConfigs: UserConfigs = merge({}, DEFAULT_USER_CONFIGS);

  constructor() {
    super();
    gAuthPromise.then((gAuth) => this.appState.gAuth = gAuth);
  }

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();

    // Sync uncommitted configs with committed configs.
    this.disposer = reaction(
      () => merge({}, this.configsStore.userConfigs),
      (committedConfig) => merge(this.uncommittedConfigs, committedConfig),
      {fireImmediately: true},
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  protected render() {
    return html`
      <mwc-dialog
        id="settings-dialog"
        heading="Settings"
        ?open=${this.showSettingsDialog}
        @closed=${(event: CustomEvent<{action: string}>) => {
          if (event.detail.action === 'save') {
            merge(this.configsStore.userConfigs, this.uncommittedConfigs);
            this.configsStore.save();
          }
          // Reset uncommitted configs.
          merge(this.uncommittedConfigs, this.configsStore.userConfigs);
          this.showSettingsDialog = false;
        }}
      >
        <label for="default-build-page-tab">Default build page tab:</label>
        <select
          id="default-build-page-tab"
          @change=${(e: InputEvent) => this.uncommittedConfigs.defaultBuildPageTabName = (e.target as HTMLOptionElement).value}
        >
          ${TAB_NAME_LABEL_TUPLES.map(([tabName, label]) => html`
          <option
            value=${tabName}
            ?selected=${tabName === this.uncommittedConfigs.defaultBuildPageTabName}
          >${label}</option>
          `)}
        </select>
        <label for="show-test-results-hints">Test Results Integration Hints:</label>
        <input
          id="show-test-results-hints"
          type="checkbox"
          ?checked=${this.uncommittedConfigs.hints.showTestResultsHint}
          @change=${(e: InputEvent) => this.uncommittedConfigs.hints.showTestResultsHint = (e.target as HTMLInputElement).checked}
        >
        <mwc-button slot="primaryAction" dialogAction="save" dense unelevated>Save</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="dismiss">Cancel</mwc-button>
      </mwc-dialog>
      <div id="container">
        <div id="title-container">
          <a href="/" id="title-link">
            <img id="chromium-icon" src="https://storage.googleapis.com/chrome-infra/lucy-small.png"/>
            <span id="headline">LUCI</span>
          </a>
        </div>
        <mwc-icon
          id="feedback"
          title="Send Feedback"
          class="interactive-icon"
          @click=${() => window.open(genFeedbackUrl())}
        >feedback</mwc-icon>
        <mwc-icon
          class="interactive-icon"
          @click=${() => this.showSettingsDialog = true}
        >settings</mwc-icon>
        <div id="signin">
          ${this.appState.gAuth ? html`
          <milo-signin
            .gAuth=${this.appState.gAuth}
            @user-update=${(e: UserUpdateEvent) => {
              this.appState.accessToken = e.detail.getAuthResponse().access_token || '';
            }}
          ></milo-signin>` : ''}
        </div>
      </div>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      --header-height: 52px;
    }

    #container {
      box-sizing: border-box;
      height: var(--header-height);
      padding: 10px 0;
      display: flex;
    }
    #title-container {
      display: flex;
      flex: 1 1 100%;
      align-items: center;
      margin-left: 14px;
    }
    #title-link {
      display: flex;
      align-items: center;
      text-decoration: none;
    }
    #chromium-icon {
      display: inline-block;
      width: 32px;
      height: 32px;
      margin-right: 8px;
    }
    #headline {
      color: var(--light-text-color);
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 18px;
      font-weight: 300;
      letter-spacing: 0.25px;
    }
    #signin {
      margin-right: 14px;
      flex-shrink: 0;
    }
    .interactive-icon {
      cursor: pointer;
      height: 32px;
      width: 32px;
      --mdc-icon-size: 28px;
      margin-right: 14px;
      position: relative;
      color: black;
      opacity: 0.6;
    }
    .interactive-icon:hover {
      opacity: 0.8;
    }
    #settings-dialog {
      --mdc-dialog-min-width: 600px;
    }
    select {
      display: block;
      width: 100%;
      padding: .375rem .75rem;
      font-size: 1rem;
      line-height: 1.5;
      background-clip: padding-box;
      border: 1px solid var(--divider-color);
      border-radius: .25rem;
      transition: border-color .15s ease-in-out,box-shadow .15s ease-in-out;
    }
    #show-test-results-hints {
      margin-top: 10px;
    }
  `;
}
