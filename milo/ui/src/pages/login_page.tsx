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
import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { Router } from '@vaadin/router';
import { customElement } from 'lit-element';
import { makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { createRoot, Root } from 'react-dom/client';

import { changeUserState } from '../components/signin';
import { consumer } from '../libs/context';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../store';
import commonStyle from '../styles/common_style.css';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - root.
 * in that order.
 */
export const LoginPage = observer(() => {
  const store = useStore();

  const isLoggedIn = ![undefined, ANONYMOUS_IDENTITY].includes(store.authState.userIdentity);

  useEffect(() => {
    if (!isLoggedIn) {
      return;
    }
    const redirect = new URLSearchParams(window.location.search).get('redirect');
    Router.go(redirect || '/');
  }, [isLoggedIn]);

  return (
    <div css={{ margin: '8px 16px' }}>
      You must{' '}
      <a onClick={() => changeUserState(true)} css={{ textDecoration: 'underline', cursor: 'pointer' }}>
        sign in
      </a>{' '}
      to see anything useful.
    </div>
  );
});

@customElement('milo-login-page')
@consumer
export class SearchPageElement extends MobxLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;

  private readonly cache: EmotionCache;
  private readonly parent: HTMLDivElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('div');
    const child = document.createElement('div');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-login-page',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <LoginPage />
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
