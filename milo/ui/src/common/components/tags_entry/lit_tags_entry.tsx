// Copyright 2024 The LUCI Authors.
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
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { createRoot, Root } from 'react-dom/client';

import { StringPair } from '@/common/services/common';
import { commonStyles } from '@/common/styles/stylesheets';

import { TagsEntry } from './tags_entry';

@customElement('milo-tags-entry')
export class TagsEntryElement extends MobxLitElement {
  @observable.ref tags!: readonly StringPair[];

  private readonly cache: EmotionCache;
  private readonly parent: HTMLSpanElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('span');
    const child = document.createElement('span');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-tags-entry',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <TagsEntry tags={this.tags} ruler="invisible" />
      </CacheProvider>,
    );
    return this.parent;
  }

  static styles = [commonStyles];
}
