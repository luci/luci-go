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
import { Router } from '@vaadin/router';
import { css, customElement, html, LitElement, property } from 'lit-element';

import '../components/signin';
import { UserUpdateEvent } from '../components/signin';
import { AppState, provideAppState } from '../context/app_state/app_state';
import { provideUserConfigs, UserConfigsStore } from '../context/app_state/user_configs';
import { router } from '../routes';

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

/**
 * Renders page header, including a sign-in widget and a feedback button, at the
 * top of the child nodes.
 * Refreshes the page when a new clientId is provided.
 */
@customElement('milo-page-layout')
@provideUserConfigs
@provideAppState
export class PageLayoutElement extends LitElement {
  @property() private gAuth: gapi.auth2.GoogleAuth | null = null;
  readonly appState = new AppState();

  readonly configStore = new UserConfigsStore();
  get userConfigs() { return this.configStore.userConfigs; }

  constructor() {
    super();
    gAuthPromise.then((gAuth) => this.gAuth = gAuth);
  }

  connectedCallback() {
    super.connectedCallback();
    this.configStore.init();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.configStore.dispose();
  }

  protected render() {
    return html`
      <div id="container">
        <div id="title-container">
          <img id="chromium-icon" src="https://storage.googleapis.com/chrome-infra/lucy-small.png"/>
          <span id="headline">LUCI</span>
        </div>
        <div
          id="feedback"
          title="Send Feedback"
          @click=${() => window.open(genFeedbackUrl())}
        >
          <mwc-icon>feedback</mwc-icon>
        </div>
        <div id="signin">
          ${this.gAuth ? html`
          <milo-signin
            .gAuth=${this.gAuth}
            @user-update=${(e: UserUpdateEvent) => {
              this.appState.accessToken = e.detail.getAuthResponse().access_token || '';
              if (!this.appState.accessToken) {
                const searchParams = new URLSearchParams();
                searchParams.set('redirect', window.location.href);
                Router.go(`${router.urlForName('login')}?${searchParams}`);
              }
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
      width: 32px;
      height: 32px;
      flex-shrink: 0;
    }
    #feedback {
      cursor: pointer;
      height: 32px;
      width: 32px;
      --mdc-icon-size: 28px;
      margin-right: 14px;
      position: relative;
      color: black;
      opacity: 0.4;
    }
    #feedback>mwc-icon {
      position: absolute;
      top: 50%;
      transform: translateY(-50%);
    }
    #feedback:hover {
      opacity: 0.6;
    }
  `;
}
