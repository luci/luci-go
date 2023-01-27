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

import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache, keyframes } from '@emotion/react';
import { Box, BoxProps, styled } from '@mui/material';
import { LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';
import { createRoot, Root } from 'react-dom/client';

import commonStyle from '../styles/common_style.css';

const bounceEffect = keyframes`
  0%,
  80%,
  100% {
    transform: scale(0);
  }
  40% {
    transform: scale(1);
  }
`;

const Dot = styled(Box)<BoxProps>(() => ({
  width: '0.75em',
  height: '0.75em',
  borderRadius: '100%',
  backgroundColor: 'currentColor',
  display: 'inline-block',
  animation: `${bounceEffect} 1.4s infinite ease-in-out both`,
}));

/**
 * A simple 3-dots loading indicator.
 */
export function DotSpinner() {
  return (
    <Box
      sx={{
        display: 'inline-block',
        textAlign: 'center',
        color: 'var(--active-text-color)',
      }}
    >
      <Dot
        sx={{
          animationDelay: '-0.32s',
        }}
      />
      <Dot
        sx={{
          animationDelay: '-0.16s',
        }}
      />
      <Dot />
    </Box>
  );
}

/**
 * A simple 3-dots loading indicator.
 */
@customElement('milo-dot-spinner')
export class DotSpinnerElement extends LitElement {
  private readonly cache: EmotionCache;
  private readonly parent: HTMLSpanElement;
  private readonly root: Root;

  constructor() {
    super();
    this.parent = document.createElement('span');
    const child = document.createElement('span');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-dot-spinner',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <DotSpinner />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
