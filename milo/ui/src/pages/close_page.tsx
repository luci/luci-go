// Copyright 2023 The LUCI Authors.
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
import { customElement } from 'lit-element';
import { createRoot, Root } from 'react-dom/client';

import commonStyle from '../styles/common_style.css';

export function ClosePage() {
  return <div css={{ margin: '20px' }}>You can close this page if it's not automatically closed</div>;
}

@customElement('milo-close-page')
export class ClosePageElement extends MobxLitElement {
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
      key: 'milo-close-page',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <ClosePage />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
