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
import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache } from '@emotion/react';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { createRoot, Root } from 'react-dom/client';

import { Changelist } from '../services/luci_analysis';
import { commonStyles } from '../styles/stylesheets';

export interface ChangelistBadgeProps {
  readonly changelists: readonly Changelist[];
}

export function getClLabel(cl: Changelist): string {
  return `c/${cl.change}/${cl.patchset}`;
}

export function getClLink(cl: Changelist): string {
  return `https://${cl.host}/c/${cl.change}/${cl.patchset}`;
}

export function ChangelistsTooltip({ changelists }: ChangelistBadgeProps) {
  return (
    <div css={{ padding: 5 }}>
      {changelists.map((cl, i) => (
        <a key={i} href={getClLink(cl)} target="_blank" rel="noreferrer">
          {getClLabel(cl)}
        </a>
      ))}
    </div>
  );
}

@customElement('milo-changelists-tooltip')
export class ChangelistsTooltipElement extends MobxLitElement {
  @observable.ref changelists!: readonly Changelist[];

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
      key: 'milo-changelists-tooltip',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <ChangelistsTooltip changelists={this.changelists} />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyles];
}
