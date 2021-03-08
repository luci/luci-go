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
import { styleMap } from 'lit-html/directives/style-map';
import { observable } from 'mobx';

import { AppState, provideAppState } from '../context/app_state';
import { provideConfigsStore, UserConfigsStore } from '../context/user_configs';
import { genFeedbackUrl } from '../libs/utils';
import './signin';
import { UserUpdateEvent } from './signin';

const gAuthPromise = new Promise<gapi.auth2.GoogleAuth>((resolve, reject) => {
  window.gapi?.load('auth2', () => {
    gapi.auth2
      .init({client_id: CONFIGS.OAUTH2.CLIENT_ID, scope: 'email'})
      .then(resolve, reject);
  });
});

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

  @observable.ref errorMsg: string | null = null;

  constructor() {
    super();
    gAuthPromise
      .then((gAuth) => this.appState.gAuth = gAuth)
      .catch(() => this.appState.accessToken = '');
  }

  errorHandler = (event: ErrorEvent) => {
    this.errorMsg = event.message;
  }

  connectedCallback() {
    super.connectedCallback();
    this.addEventListener('error', this.errorHandler);
  }

  disconnectedCallback() {
    this.removeEventListener('error', this.errorHandler);
    super.disconnectedCallback();
  }

  protected render() {
    return html`
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
          title="Settings"
          @click=${() => this.appState.showSettingsDialog = true}
          style=${styleMap({display: this.appState.hasSettingsDialog > 0 ? '' : 'none'})}
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
      ${this.errorMsg === null ?
      html`<slot></slot>` :
      html`
      <div id="error-label">An error occurred:</div>
      <div id="error-message">
        ${this.errorMsg.split('\n').map((line) => html`<p>${line}</p>`)}
      </div>
      `
      }
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
      margin-top: 2px;
      margin-right: 14px;
      position: relative;
      color: black;
      opacity: 0.6;
    }
    .interactive-icon:hover {
      opacity: 0.8;
    }

    #error-label {
      margin: 8px 16px;
    }

    #error-message {
      margin: 8px 16px;
      background-color: var(--block-background-color);
      padding: 5px;
    }
  `;
}
