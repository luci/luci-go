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
import { Box, FormControl, InputAdornment, MenuItem, Select, TextField } from '@mui/material';
import { customElement } from 'lit/decorators.js';
import { debounce } from 'lodash-es';
import { makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useCallback, useEffect, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import { consumer } from '../../libs/context';
import { URLExt } from '../../libs/utils';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../../store';
import { DEFAULT_SEARCH_TARGET, DEFAULT_TEST_PROJECT, SearchTarget } from '../../store/search_page';
import commonStyle from '../../styles/common_style.css';
import { BuilderList } from './builder_list';
import { TestList } from './test_list';

const SEARCH_DELAY = Object.freeze({
  [SearchTarget.Builders]: 300,
  // Use a longer delay for tests because it triggers actual queries to the
  // server.
  [SearchTarget.Tests]: 600,
});

const SEARCH_HINT = Object.freeze({
  [SearchTarget.Builders]: 'Search builders in all projects',
  [SearchTarget.Tests]: 'Search tests in the specified project',
});

export const SearchPage = observer(() => {
  const pageState = useStore().searchPage;
  const [searchQuery, setSearchQuery] = useState(pageState.searchQuery);
  const [project, setProject] = useState(pageState.testProject);

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search);

    const t = searchParams.get('t');
    if (t !== null && Object.values(SearchTarget).includes(t as SearchTarget)) {
      pageState.setSearchTarget(t as SearchTarget);
    }

    const tp = searchParams.get('tp');
    if (tp !== null) {
      pageState.setTestProject(tp);
      setProject(tp);
    }

    const q = searchParams.get('q');
    if (q !== null) {
      pageState.setSearchQuery(q);
      setSearchQuery(q);
    }
  }, [pageState]);

  useEffect(() => {
    const url = new URLExt(window.location.href)
      .setSearchParam('t', pageState.searchTarget)
      .setSearchParam('tp', pageState.testProject)
      .setSearchParam('q', pageState.searchQuery)
      // Make the URL shorter.
      .removeMatchedParams({ t: DEFAULT_SEARCH_TARGET, tp: DEFAULT_TEST_PROJECT, q: '' });
    window.history.replaceState(null, '', url);
  }, [pageState.searchTarget, pageState.testProject, pageState.searchQuery]);

  // Update the search query in the pageStore after a slight delay to avoid
  // updating the list or triggering network requests too frequently.
  const executeSearch = useCallback(
    debounce(
      (newSearchQuery: string) => pageState.setSearchQuery(newSearchQuery),
      SEARCH_DELAY[pageState.searchTarget]
    ),
    [pageState, pageState.searchTarget]
  );

  useEffect(() => {
    if (searchQuery !== pageState.searchQuery) {
      executeSearch(searchQuery);
    }
    // Execute search cancel the previous call automatically when the next call
    // is scheduled. However, when the search target is changed, executeSearch
    // itself is updated. Therefore we need to cancel the search explicitly.
    return () => executeSearch.cancel();
  }, [executeSearch, searchQuery]);

  const searchTarget = pageState.searchTarget;

  return (
    <Box sx={{ px: 6, py: 5 }}>
      <Box sx={{ display: 'grid', mx: 20, gridTemplateColumns: 'auto auto 1fr' }}>
        <Select
          value={searchTarget}
          onChange={(e) => pageState.setSearchTarget(e.target.value as SearchTarget)}
          MenuProps={{ disablePortal: true }}
          size="small"
          sx={{
            '& .MuiOutlinedInput-notchedOutline': {
              borderRightColor: 'transparent',
              borderTopRightRadius: 0,
              borderBottomRightRadius: 0,
            },
          }}
        >
          <MenuItem value={SearchTarget.Builders}>Builders</MenuItem>
          <MenuItem value={SearchTarget.Tests}>Tests</MenuItem>
        </Select>
        {searchTarget === SearchTarget.Tests ? (
          <TextField
            label="project"
            value={project}
            onChange={(e) => setProject(e.target.value)}
            onKeyDown={(e) => {
              if (!['Tab', 'Enter'].includes(e.key)) {
                return;
              }
              pageState.setTestProject(project);
            }}
            onBlur={() => pageState.setTestProject(project)}
            variant="outlined"
            size="small"
            sx={{
              width: '120px',
              marginLeft: '-1px',
              '& .MuiOutlinedInput-notchedOutline': {
                borderRightColor: 'transparent',
                borderRadius: 0,
              },
            }}
          />
        ) : (
          <Box></Box>
        )}
        <FormControl>
          <TextField
            value={searchQuery}
            placeholder={SEARCH_HINT[searchTarget]}
            onChange={(e) => setSearchQuery(e.target.value)}
            autoFocus
            variant="outlined"
            size="small"
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search />
                </InputAdornment>
              ),
            }}
            sx={{
              marginLeft: '-1px',
              '& .MuiOutlinedInput-notchedOutline': {
                borderTopLeftRadius: 0,
                borderBottomLeftRadius: 0,
              },
            }}
          />
        </FormControl>
      </Box>
      <Box sx={{ mt: 5 }}>{searchTarget === SearchTarget.Builders ? <BuilderList /> : <TestList />}</Box>
    </Box>
  );
});

@customElement('milo-search-page')
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
      key: 'milo-search-page',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <SearchPage></SearchPage>
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
