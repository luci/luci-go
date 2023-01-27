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
import { Chip } from '@mui/material';
import { html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { createRef } from 'react';
import { createRoot, Root } from 'react-dom/client';

import './changelists_tooltip';
import { Changelist } from '../services/luci_analysis';
import commonStyle from '../styles/common_style.css';
import { getClLabel, getClLink } from './changelists_tooltip';
import { HideTooltipEventDetail, ShowTooltipEventDetail } from './tooltip';

export interface ChangelistBadgeProps {
  readonly changelists: readonly Changelist[];
}

export function ChangelistsBadge({ changelists }: ChangelistBadgeProps) {
  const badgeRef = createRef<HTMLAnchorElement>();
  const firstCl = changelists[0];
  if (!firstCl) {
    return <></>;
  }

  const hasMultipleCls = changelists.length > 1;

  return (
    <Chip
      label={`${getClLabel(firstCl)}${hasMultipleCls ? ', ...' : ''}`}
      size="small"
      component="a"
      target="_blank"
      href={getClLink(firstCl)}
      clickable
      ref={badgeRef}
      onMouseOver={() => {
        if (!hasMultipleCls) {
          return;
        }
        const tooltip = document.createElement('div');
        render(html`<milo-changelists-tooltip .changelists=${changelists}></milo-changelists-tooltip>`, tooltip);
        window.dispatchEvent(
          new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
            detail: {
              tooltip,
              targetRect: badgeRef.current!.getBoundingClientRect(),
              gapSize: 2,
            },
          })
        );
      }}
      onMouseOut={() => {
        if (!hasMultipleCls) {
          return;
        }
        window.dispatchEvent(new CustomEvent<HideTooltipEventDetail>('hide-tooltip', { detail: { delay: 50 } }));
      }}
      onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => e.stopPropagation()}
    />
  );
}

@customElement('milo-changelists-badge')
export class ChangelistsBadgeElement extends MobxLitElement {
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
      key: 'milo-changelists-badge',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <ChangelistsBadge changelists={this.changelists} />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
