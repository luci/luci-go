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
import { Feedback, Settings } from '@mui/icons-material';
import { Box, Link, LinkProps, styled } from '@mui/material';
import { customElement } from 'lit-element';
import { makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { createRoot, Root } from 'react-dom/client';

import { consumer } from '../libs/context';
import { genFeedbackUrl } from '../libs/utils';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../store';
import commonStyle from '../styles/common_style.css';
import { SignIn } from './signin';

const INTERACTIVE_ICON_STYLE = {
  cursor: 'pointer',
  height: '32px',
  width: '32px',
  '--mdc-icon-size': '28px',
  marginTop: '2px',
  marginRight: '14px',
  position: 'relative',
  color: 'black',
  opacity: '0.6',
  '&:hover': {
    opacity: 0.8,
  },
};

const NavLink = styled(Link)<LinkProps>(() => ({
  color: 'var(--default-text-color)',
  textDecoration: 'underline',
  cursor: 'pointer',
}));

export const TopBar = observer(() => {
  const store = useStore();

  return (
    <Box
      sx={{
        boxSizing: 'border-box',
        height: '52px',
        padding: '10px 0',
        display: 'flex',
      }}
    >
      <Box
        sx={{
          flex: '1 1 100%',
          alignItems: 'center',
          verticalAlign: 'center',
          marginLeft: '14px',
          lineHeight: '32px',
        }}
      >
        <NavLink href="/">Home</NavLink> | <NavLink href="/search">Search</NavLink>
      </Box>
      <Feedback onClick={() => window.open(genFeedbackUrl())} sx={INTERACTIVE_ICON_STYLE} />
      {store.hasSettingsDialog > 0 ? (
        <Settings onClick={() => store.setShowSettingsDialog(true)} sx={INTERACTIVE_ICON_STYLE} />
      ) : (
        <></>
      )}
      <Box
        sx={{
          marginRight: '14px',
          flexShrink: 0,
        }}
      >
        {store.authState.value ? (
          <SignIn
            identity={store.authState.value.identity}
            email={store.authState.value.email}
            picture={store.authState.value.picture}
          />
        ) : (
          <></>
        )}
      </Box>
    </Box>
  );
});

@customElement('milo-top-bar')
@consumer
export class TopBarElement extends MobxLitElement {
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
      key: 'milo-top-bar',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <TopBar />
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
