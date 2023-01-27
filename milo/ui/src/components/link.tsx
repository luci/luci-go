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
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { createRoot, Root } from 'react-dom/client';

import { Link } from '../models/link';
import commonStyle from '../styles/common_style.css';

export interface MiloLinkProps {
  readonly link: Link;
  readonly target?: string;
}

export const MiloLink = observer(({ link, target }: MiloLinkProps) => {
  return (
    <a href={link.url} aria-label={link.ariaLabel} target={target || ''}>
      {link.label}
    </a>
  );
});

/**
 * Renders a Link object.
 */
@customElement('milo-link')
export class MiloLinkElement extends MobxLitElement {
  @observable.ref link!: Link;
  @observable.ref target?: string;

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
      key: 'milo-link',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <MiloLink link={this.link} target={this.target} />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
