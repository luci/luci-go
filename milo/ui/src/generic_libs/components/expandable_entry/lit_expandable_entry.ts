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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { makeObservable, observable } from 'mobx';

/**
 * Renders an expandable entry.
 */
// Keep a separate implementation instead of wrapping the React component so
// 1. we can catch events originated from shadow-dom, and
// 2. the rendering performance is as good as possible (there could be > 10,000
// entries rendered on the screen).
@customElement('milo-expandable-entry')
export class ExpandableEntryElement extends MobxLitElement {
  /**
   * Configure whether the content ruler should be rendered.
   * * visible: the default option. Renders the content ruler.
   * * invisible: hide the content ruler but keep the indentation.
   * * none: hide the content ruler and don't keep the indentation.
   */
  @observable.ref contentRuler: 'visible' | 'invisible' | 'none' = 'visible';

  onToggle = (_isExpanded: boolean) => {
    /* do nothing by default */
  };

  @observable.ref private _expanded = false;
  get expanded() {
    return this._expanded;
  }
  set expanded(isExpanded) {
    if (isExpanded === this._expanded) {
      return;
    }
    this._expanded = isExpanded;
    this.onToggle(this._expanded);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <div
        id="expandable-header"
        @click=${() => (this.expanded = !this.expanded)}
      >
        <mwc-icon>${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <slot name="header"></slot>
      </div>
      <div
        id="body"
        style=${styleMap({
          'grid-template-columns':
            this.contentRuler === 'none' ? '1fr' : '24px 1fr',
        })}
      >
        <div
          id="content-ruler"
          style=${styleMap({
            display: this.contentRuler === 'none' ? 'none' : '',
            visibility: this.contentRuler === 'invisible' ? 'hidden' : '',
          })}
        ></div>
        <slot
          name="content"
          style=${styleMap({ display: this.expanded ? '' : 'none' })}
        ></slot>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: block;
      --header-height: 24px;
    }

    #expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: var(--header-height);
      grid-gap: 5px;
      cursor: pointer;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      border-left: 1px solid var(--divider-color);
      width: 0px;
      margin-left: 11.5px;
    }
  `;
}
