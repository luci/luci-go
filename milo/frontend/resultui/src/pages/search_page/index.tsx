// Copyright 2022 The LUCI Authors.
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
import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache } from '@emotion/react';
import { customElement } from 'lit-element';
import { observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import React from 'react';
import { render } from 'react-dom';

import { StoreProvider, useStore } from '../../components/StoreProvider';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumer } from '../../libs/context';

export const SearchPage = observer(() => {
  const appState = useStore().appState;
  const pageState = useStore().searchPage;
  const onChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    pageState.setSearchQuery(e.target.value);
  };
  return (
    <>
      <div>{appState.authState?.identity}</div>
      <input value={pageState.searchQuery} onChange={onChange}></input>
      <div>{pageState.searchQuery}</div>
    </>
  );
});

@customElement('milo-search-page')
@consumer
export class SearchPageElement extends MobxLitElement {
  @observable.ref @consumeAppState() appState!: AppState;

  private readonly cache: EmotionCache;
  private readonly parent: HTMLDivElement;
  private readonly child: HTMLDivElement;

  constructor() {
    super();
    this.parent = document.createElement('div');
    this.child = document.createElement('div');
    this.parent.appendChild(this.child);
    this.cache = createCache({
      key: 'milo-search-page',
      container: this.parent,
    });
  }

  protected render() {
    render(
      <CacheProvider value={this.cache}>
        <StoreProvider appState={this.appState}>
          <SearchPage></SearchPage>
        </StoreProvider>
      </CacheProvider>,
      this.child
    );
    return this.parent;
  }
}
