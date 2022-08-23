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
import { FormControl, InputAdornment, MenuItem, Select, TextField } from '@mui/material';
import { customElement } from 'lit-element';
import { observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useEffect, useRef, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import '../../components/dot_spinner';
import { StoreProvider, useStore } from '../../components/StoreProvider';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumer } from '../../libs/context';
import { SearchTarget } from '../../store/search_page';
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

  const timeoutId = useRef(0);
  useEffect(() => () => window.clearTimeout(timeoutId.current), []);

  const searchTarget = pageState.searchTarget;

  return (
    <div css={{ padding: '20px 30px' }}>
      <FormControl
        css={{
          display: 'grid',
          margin: '0 200px',
          gridTemplateColumns: '110px auto 1fr',
        }}
      >
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
            onChange={(e) => {
              setProject(e.target.value);
            }}
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
          <div></div>
        )}
        <TextField
          value={searchQuery}
          placeholder={SEARCH_HINT[searchTarget]}
          onChange={(e) => {
            setSearchQuery(e.target.value);

            // Update the search query after a slight delay to avoid updating
            // the list too frequently.
            window.clearTimeout(timeoutId.current);
            timeoutId.current = window.setTimeout(
              () => pageState.setSearchQuery(e.target.value),
              SEARCH_DELAY[searchTarget]
            );
          }}
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
      <div css={{ marginTop: '20px' }}>
        {searchTarget === SearchTarget.Builders ? (
          <>
            <BuilderList groupedBuilders={pageState.groupedBuilders} />
            {(pageState.builderLoader?.isLoading ?? true) && (
              <span>
                Loading <milo-dot-spinner></milo-dot-spinner>
              </span>
            )}
          </>
        ) : (
          <TestList
            project={pageState.testProject}
            searchQuery={pageState.searchQuery}
            testLoader={pageState.testLoader}
          />
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
