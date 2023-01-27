// Copyright 2021 The LUCI Authors.
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

import '@material/mwc-menu';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable } from 'mobx';

import './drag_tracker';
import commonStyle from '../styles/common_style.css';
import { DragEvent } from './drag_tracker';

@customElement('milo-column-header')
export class ColumnHeaderElement extends MobxLitElement {
  @observable.ref label!: string;
  @observable.ref tooltip!: string;

  // finalized is set to true when the user stopped dragging.
  @observable.ref resizeColumn?: (delta: number, finalized: boolean) => void;
  @observable.ref sortByColumn?: (ascending: boolean) => void;
  @observable.ref groupByColumn?: () => void;
  @observable.ref hideColumn?: () => void;

  @observable.ref private menuIsOpen = false;

  @computed private get canResize() {
    return Boolean(this.resizeColumn);
  }
  @computed private get canSort() {
    return Boolean(this.sortByColumn);
  }
  @computed private get canGroup() {
    return Boolean(this.groupByColumn);
  }
  @computed private get canHide() {
    return Boolean(this.hideColumn);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private renderResizer() {
    if (!this.canResize) {
      return html``;
    }
    let widthDiff = 0;
    return html`
      <milo-drag-tracker
        id="resizer"
        @drag=${(e: DragEvent) => {
          widthDiff = e.detail.dx;
          this.resizeColumn!(widthDiff, false);
        }}
        @dragend=${() => this.resizeColumn!(widthDiff, true)}
      ></milo-drag-tracker>
    `;
  }

  protected render() {
    return html`
      <div id="prop-label" title=${this.tooltip} @click=${() => (this.menuIsOpen = !this.menuIsOpen)}>
        ${this.label}
      </div>
      <div id="padding"></div>
      ${this.renderResizer()}
      <mwc-menu x="0" y="20" ?open=${this.menuIsOpen} @closed=${() => (this.menuIsOpen = false)}>
        <mwc-list-item
          style=${styleMap({ display: this.canSort ? '' : 'none' })}
          @click=${() => this.sortByColumn?.(true)}
        >
          Sort in ascending order
        </mwc-list-item>
        <mwc-list-item
          style=${styleMap({ display: this.canSort ? '' : 'none' })}
          @click=${() => this.sortByColumn?.(false)}
        >
          Sort in descending order
        </mwc-list-item>
        <mwc-list-item
          style=${styleMap({ display: this.canGroup ? '' : 'none' })}
          @click=${() => this.groupByColumn?.()}
        >
          Group by this column
        </mwc-list-item>
        <mwc-list-item style=${styleMap({ display: this.canHide ? '' : 'none' })} @click=${() => this.hideColumn?.()}>
          Hide column
        </mwc-list-item>
      </mwc-menu>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: flex;
        position: relative;
      }

      #prop-label {
        flex: 0 1 auto;
        cursor: pointer;
        color: var(--active-text-color);
        overflow: hidden;
        text-overflow: ellipsis;
      }
      #padding {
        flex: 1 1 auto;
      }
      #resizer {
        flex: 0 0 auto;
        background: linear-gradient(var(--divider-color), var(--divider-color)) 2px 0/1px 100% no-repeat;
        width: 5px;
        cursor: col-resize;
      }
    `,
  ];
}
