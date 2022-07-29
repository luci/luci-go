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
import { Search } from '@mui/icons-material';
import { FormControl, InputAdornment, TextField } from '@mui/material';
import { customElement } from 'lit-element';
import { observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useEffect, useRef, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import '../../components/dot_spinner';
import { StoreProvider, useStore } from '../../components/StoreProvider';
import { AppState, consumeAppState } from '../../context/app_state';
import { LoadingState } from '../../libs/constants';
import { consumer } from '../../libs/context';
import commonStyle from '../../styles/common_style.css';
import { BuildersList } from './builders_list';

export const SearchPage = observer(() => {
  const pageState = useStore().searchPage;
  const [searchQuery, setSearchQuery] = useState(pageState.searchQuery);

  const timeoutId = useRef(0);
  useEffect(() => () => window.clearTimeout(timeoutId.current), []);

  return (
    <div css={{ padding: '20px 30px' }}>
      <FormControl css={{ margin: '0px 200px', width: 'calc(100% - 400px)' }}>
        <TextField
          id="failure_filter"
          value={searchQuery}
          placeholder="Search builders in all projects"
          onChange={(e) => {
            setSearchQuery(e.target.value);

            // Update the search query after a light delay to avoid updating the
            // list too frequently.
            window.clearTimeout(timeoutId.current);
            timeoutId.current = window.setTimeout(() => pageState.setSearchQuery(e.target.value), 300);
          }}
          autoFocus
          fullWidth
          variant="outlined"
          size="small"
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search />
              </InputAdornment>
            ),
          }}
        ></TextField>
      </FormControl>
      <div css={{ marginTop: '20px' }}>
        <BuildersList groupedBuilders={pageState.groupedBuilders} />
        {pageState.loadingBuildersState !== LoadingState.Fulfilled && (
          <span>
            Loading <milo-dot-spinner></milo-dot-spinner>
          </span>
        )}
      </div>
    </div>
  );
});

@customElement('milo-search-page')
@consumer
export class SearchPageElement extends MobxLitElement {
  @observable.ref @consumeAppState() appState!: AppState;

  private readonly cache: EmotionCache;
  private readonly parent: HTMLDivElement;
  private readonly root: Root;

  constructor() {
    super();
    this.parent = document.createElement('div');
    const child = document.createElement('div');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-search-page',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider appState={this.appState}>
          <SearchPage></SearchPage>
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
