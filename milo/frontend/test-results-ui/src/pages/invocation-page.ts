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
import * as signin from '@chopsui/chops-signin';
import { BeforeEnterObserver, RouterLocation, PreventAndRedirectCommands, Router } from '@vaadin/router';
import { customElement, html } from 'lit-element';
import { action, observable } from 'mobx';

import '../components/page-header';

/**
 * Main test results page.
 * Read invocation_name from params.
 * If not logged in, redirect to '/login?redirect=${current_url}'.
 * If invocation_name not provided, redirect to '/not-found'.
 * Otherwise, show invocation.
 */
@customElement('tr-invocation-page')
export class InvocationPageElement extends MobxLitElement implements BeforeEnterObserver {
    @observable.ref
    accessToken = '';
    @observable.ref
    invocationName = '';

    protected render() {
        return html`
        <tr-page-header></tr-page-header>
        <div>${this.invocationName}</div>
        `;
    }

    async onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
        await this.refreshAccessToken();
        const invocationName = location.params['invocation_name'];
        if (typeof invocationName !== 'string') {
            return cmd.redirect('/not-found');
        }
        this.invocationName = invocationName;
    }

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('user-update', this.refreshAccessToken);
        this.refreshAccessToken();
    }
    disconnectedCallback() {
        super.disconnectedCallback();
        window.removeEventListener('user-update', this.refreshAccessToken);
    }
    @action
    private refreshAccessToken = async () => {
        this.accessToken = signin
            .getAuthInstanceSync()
            ?.currentUser
            .get()
            .getAuthResponse()
            .access_token || '';
        if (!this.accessToken) {
            const searchParams = new URLSearchParams()
            searchParams.set('redirect', window.location.href)
            return Router.go(`/login?${searchParams}`);
        }
    }
}
