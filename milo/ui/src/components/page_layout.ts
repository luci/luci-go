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

import { Router } from '@vaadin/router';
import { BroadcastChannel } from 'broadcast-channel';
import { css, customElement, html } from 'lit-element';
import { destroy } from 'mobx-state-tree';

import './tooltip';
import './top_bar';
import { MAY_REQUIRE_SIGNIN, OPTIONAL_RESOURCE } from '../common_tags';
import { provider } from '../libs/context';
import { errorHandler, handleLocally } from '../libs/error_handler';
import { ProgressiveNotifier, provideNotifier } from '../libs/observer_element';
import { hasTags } from '../libs/tag';
import { createStaticTrustedURL } from '../libs/utils';
import { router } from '../routes';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { provideStore, Store } from '../store';
import commonStyle from '../styles/common_style.css';
import { MiloBaseElement } from './milo_base';

export const refreshAuthChannel = new BroadcastChannel('refresh-auth-channel');

function redirectToLogin(err: ErrorEvent, ele: PageLayoutElement) {
  if (
    ele.store.authState.value?.identity === ANONYMOUS_IDENTITY &&
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
export class PageLayoutElement extends MiloBaseElement {
  @provideStore({ global: true }) readonly store = Store.create();
  @provideNotifier({ global: true }) readonly notifier = new ProgressiveNotifier({
    // Ensures that everything above the current scroll view is rendered.
    // This reduces page shifting due to incorrect height estimate.
    rootMargin: '1000000px 0px 0px 0px',
  });

  connectedCallback() {
    super.connectedCallback();

    this.store.authState.init();
    if (navigator.serviceWorker && ENABLE_UI_SW) {
      this.store.workbox.init(createStaticTrustedURL('sw-js-static', '/ui/service-worker.js'));
    }

    if (navigator.serviceWorker && !document.cookie.includes('showNewBuildPage=false')) {
      navigator.serviceWorker
        .register(
          // cast to string because TypeScript doesn't allow us to use
          // TrustedScriptURL here
          createStaticTrustedURL('root-sw-js-static', '/root-sw.js') as string
        )
        .then((registration) => {
          this.store.setRedirectSw(registration);
        });
    } else {
      this.store.setRedirectSw(null);
    }

    const onRefreshAuth = () => this.store.authState.scheduleUpdate(true);
    refreshAuthChannel.addEventListener('message', onRefreshAuth);
    this.addDisposer(() => refreshAuthChannel.removeEventListener('message', onRefreshAuth));

    this.addDisposer(() => {
      destroy(this.store);
    });
  }

  protected render() {
    return html`
      <milo-tooltip></milo-tooltip>
      ${this.store.banners.map((banner) => html`<div class="banner-container">${banner}</div>`)}
      <milo-top-bar></milo-top-bar>
      <slot></slot>
    `;
  }

  static styles = [
    commonStyle,
    css`
      .banner-container {
        width: 100%;
        box-sizing: border-box;
        background-color: #feb;
        padding: 3px;
        text-align: center;
        font-weight: bold;
        font-size: 12px;
      }
    `,
  ];
}
