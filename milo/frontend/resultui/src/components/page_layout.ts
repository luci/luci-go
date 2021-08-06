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
import '@material/mwc-icon-button';
import '@material/mwc-snackbar';
import { BeforeEnterObserver, Router } from '@vaadin/router';
import { BroadcastChannel } from 'broadcast-channel';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { observable, reaction } from 'mobx';

import './signin';
import './tooltip';
import { getAuthStateCache, setAuthStateCache } from '../auth_state_cache';
import { MAY_REQUIRE_SIGNIN, OPTIONAL_RESOURCE } from '../common_tags';
import { AppState, provideAppState } from '../context/app_state';
import { provideConfigsStore, UserConfigsStore } from '../context/user_configs';
import { NEW_MILO_VERSION_EVENT_TYPE } from '../libs/constants';
import { provider } from '../libs/context';
import { errorHandler, handleLocally } from '../libs/error_handler';
import { ProgressiveNotifier, provideNotifier } from '../libs/observer_element';
import { hasTags } from '../libs/tag';
import { genFeedbackUrl, timeout } from '../libs/utils';
import { router } from '../routes';
import { ANONYMOUS_IDENTITY, queryAuthState } from '../services/milo_internal';
import commonStyle from '../styles/common_style.css';
import { MiloBaseElement } from './milo_base';

export const refreshAuthChannel = new BroadcastChannel('refresh-auth-channel');

function redirectToLogin(err: ErrorEvent, ele: PageLayoutElement) {
  if (
    ele.appState.authState?.identity === ANONYMOUS_IDENTITY &&
    hasTags(err.error, MAY_REQUIRE_SIGNIN) &&
    !hasTags(err.error, OPTIONAL_RESOURCE)
  ) {
    Router.go(`${router.urlForName('login')}?${new URLSearchParams([['redirect', window.location.href]])}`);
    return false;
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
export class PageLayoutElement extends MiloBaseElement implements BeforeEnterObserver {
  @provideAppState({ global: true }) readonly appState = new AppState();
  @provideConfigsStore({ global: true }) readonly configsStore = new UserConfigsStore();
  @provideNotifier({ global: true }) readonly notifier = new ProgressiveNotifier({
    // Ensures that everything above the current scroll view is rendered.
    // This reduces page shifting due to incorrect height estimate.
    rootMargin: '1000000px 0px 0px 0px',
  });

  @observable.ref showUpdateBanner = false;

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

  connectedCallback() {
    super.connectedCallback();

    const onNewMiloVersion = () => (this.showUpdateBanner = true);
    window.addEventListener(NEW_MILO_VERSION_EVENT_TYPE, onNewMiloVersion);
    this.addDisposer(() => window.removeEventListener(NEW_MILO_VERSION_EVENT_TYPE, onNewMiloVersion));

    const onRefreshAuth = () => this.scheduleAuthStateUpdate(true);
    refreshAuthChannel.addEventListener('message', onRefreshAuth);
    this.addDisposer(() => refreshAuthChannel.removeEventListener('message', onRefreshAuth));

    this.addDisposer(() => {
      this.appState.dispose();
      this.configsStore.dispose();
    });

    let firstUpdate = true;
    getAuthStateCache()
      .then((authState) => (this.appState.authState = authState))
      .finally(() => {
        if (!this.isConnected) {
          return;
        }

        this.addDisposer(
          reaction(
            () => this.appState.authState,
            () => {
              const wasFirstUpdate = firstUpdate;
              firstUpdate = false;
              // Cookie could be updated when the page was offline. Update the
              // auth state immediately in the first update schedule.
              this.scheduleAuthStateUpdate(wasFirstUpdate);
            },
            {
              fireImmediately: true,
              // Ensure there are at least 10s between updates. So the backend
              // returning short-lived tokens won't cause the update action to
              // fire rapidly.
              // Note: the delay is not applied to the first call.
              delay: 10000,
            }
          )
        );
      });
  }

  // A unique reference that functions as the ID of the last
  // this.scheduleAuthStateUpdate call.
  private lastScheduleId = {};

  /**
   * Updates the auth state when before it expires. When called multiple times,
   * only the last call is respected.
   *
   * @param forceUpdate when set to true, update the auth state immediately.
   */
  private async scheduleAuthStateUpdate(forceUpdate = false) {
    const scheduleId = {};
    this.lastScheduleId = scheduleId;

    let validDuration = 0;
    if (!forceUpdate && this.appState.authState !== null) {
      if (!this.appState.authState.accessTokenExpiry) {
        return;
      }
      // Refresh the access token 10s earlier to prevent the token from
      // expiring before the new token is returned.
      validDuration = this.appState.authState.accessTokenExpiry * 1000 - Date.now() - 10000;
    }

    await timeout(validDuration);

    if (!this.isConnected) {
      return;
    }

    const newAuthState = await queryAuthState();

    // There's another scheduled update. Abort the current one.
    if (this.lastScheduleId !== scheduleId) {
      return;
    }

    setAuthStateCache(newAuthState);
    this.appState.authState = newAuthState;
  }

  protected render() {
    return html`
      <mwc-snackbar labelText="A New Version of Milo is Available" timeoutMs=${-1} ?open=${this.showUpdateBanner}>
        <mwc-button
          slot="action"
          @click=${async () => {
            this.showUpdateBanner = false;
            (await window.SW_PROMISE).messageSkipWaiting();
          }}
        >
          Update
        </mwc-button>
        <mwc-icon-button icon="close" slot="dismiss" @click=${() => (this.showUpdateBanner = false)}></mwc-icon-button>
      </mwc-snackbar>
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
          ${this.appState.authState
            ? html`<milo-signin
                .identity=${this.appState.authState.identity}
                .email=${this.appState.authState.email}
                .picture=${this.appState.authState.picture}
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
      #container {
        box-sizing: border-box;
        height: 52px;
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
