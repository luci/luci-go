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
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { BeforeEnterObserver, Router } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';

import './signin';
import './tooltip';
import { AppState, provideAppState } from '../context/app_state';
import { provideConfigsStore, UserConfigsStore } from '../context/user_configs';
import { provider } from '../libs/context';
import { errorHandler, handleLocally } from '../libs/error_handler';
import { ProgressiveNotifier, provideNotifier } from '../libs/observer_element';
import { genFeedbackUrl } from '../libs/utils';
import { router } from '../routes';
import commonStyle from '../styles/common_style.css';
import { UserUpdateEvent } from './signin';

const gAuthPromise = new Promise<gapi.auth2.GoogleAuth>((resolve, reject) => {
  window.gapi?.load('auth2', () => {
    gapi.auth2.init({ client_id: CONFIGS.OAUTH2.CLIENT_ID, scope: 'email' }).then(resolve, reject);
  });
});

function redirectToLogin(err: ErrorEvent, ele: PageLayoutElement) {
  // TODO(weiweilin): add integration tests to ensure redirection works properly.
  if (err.error instanceof GrpcError) {
    const mayRequireSignin = [RpcCode.NOT_FOUND, RpcCode.PERMISSION_DENIED, RpcCode.UNAUTHENTICATED].includes(
      err.error.code
    );
    if (mayRequireSignin && ele.appState.userId === '') {
      Router.go(`${router.urlForName('login')}?${new URLSearchParams([['redirect', window.location.href]])}`);
      return false;
    }
  }
  return handleLocally(err, ele);
}

/**
 * Renders page header, including a sign-in widget, a settings button, and a
 * feedback button, at the top of the child nodes.
 * Refreshes the page when a new clientId is provided.
 */
@customElement('milo-page-layout')
@errorHandler(redirectToLogin)
@provider
export class PageLayoutElement extends MobxLitElement implements BeforeEnterObserver {
  @provideAppState readonly appState = new AppState();
  @provideConfigsStore readonly configsStore = new UserConfigsStore();
  @provideNotifier readonly notifier = new ProgressiveNotifier();

  constructor() {
    super();
    // Expires the token slightly (10s) earlier so an expired token won't be
    // used if gAuth takes a while to return the new access token.
    // We can't solely rely on SW to check the expiration because an expired
    // state could be used if <milo-page-layout /> is reinitialized at a later
    // stage.
    if (CACHED_AUTH_STATE && CACHED_AUTH_STATE.expiresAt > Date.now() + 10000) {
      this.appState.accessToken = CACHED_AUTH_STATE.accessToken;
      this.appState.userId = CACHED_AUTH_STATE.userId;
    }

    gAuthPromise
      .then((gAuth) => (this.appState.gAuth = gAuth))
      .catch(() => {
        this.appState.userId = '';
        this.appState.accessToken = '';
      });
  }

  onBeforeEnter() {
    if ('serviceWorker' in navigator) {
      // onBeforeEnter can be async.
      // But we don't want to block the rest of the page from rendering.
      navigator.serviceWorker.getRegistration('/').then((redirectSw) => {
        this.appState.redirectSw = redirectSw;
      });
    } else {
      this.appState.redirectSw = undefined;
    }
  }

  disconnectedCallback() {
    this.appState.dispose();
    super.disconnectedCallback();
  }

  protected render() {
    return html`
      <milo-tooltip></milo-tooltip>
      <div id="container">
        <div id="title-container">
          <a href="/" id="title-link">
            <img id="chromium-icon" src="https://storage.googleapis.com/chrome-infra/lucy-small.png" />
            <span id="headline">LUCI</span>
          </a>
        </div>
        <mwc-icon
          id="feedback"
          title="Send Feedback"
          class="interactive-icon"
          @click=${() => window.open(genFeedbackUrl())}
          >feedback</mwc-icon
        >
        <mwc-icon
          class="interactive-icon"
          title="Settings"
          @click=${() => (this.appState.showSettingsDialog = true)}
          style=${styleMap({ display: this.appState.hasSettingsDialog > 0 ? '' : 'none' })}
          >settings</mwc-icon
        >
        <div id="signin">
          ${this.appState.gAuth
            ? html` <milo-signin
                .gAuth=${this.appState.gAuth}
                @user-update=${async (e: UserUpdateEvent) => {
                  const authResponse = e.detail.getAuthResponse();
                  const accessToken = authResponse.access_token || '';
                  const userId = e.detail.isSignedIn() ? e.detail.getId() : '';
                  this.appState.accessToken = accessToken;
                  this.appState.userId = userId;
                  const authState: AuthState = {
                    accessToken,
                    userId,
                    // authResponse.expires_at is undefined when user is logged
                    // out. We can cache this indefinitely.
                    expiresAt: authResponse.expires_at ?? Infinity,
                  };
                  (await window.SW_PROMISE).messageSW({ type: 'SET_AUTH_STATE', authState });
                }}
              ></milo-signin>`
            : ''}
        </div>
      </div>
      <slot></slot>
    `;
  }

  static styles = [
    commonStyle,
    css`
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
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
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
        margin-top: 2px;
        margin-right: 14px;
        position: relative;
        color: black;
        opacity: 0.6;
      }
      .interactive-icon:hover {
        opacity: 0.8;
      }
    `,
  ];
}
