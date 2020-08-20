// Copyright 2020 The LUCI Authors.
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
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { observable } from 'mobx';


/**
 * An expandable panel.
 * With a clickable and draggable handle.
 * Clicking the handle can expand or collapse the panel.
 * Dragging the handle when the panel is expanded can resize the panel.
 */
@customElement('milo-left-panel')
export class LeftPanelElement extends MobxLitElement {
  @observable.ref private expanded = false;

  private isResizing = false;
  private startPos = 0;
  private startWidth = 0;
  private panelEle!: HTMLElement;

  onResize = (event: MouseEvent) => {
    const dx = event.x - this.startPos;
    if (Math.abs(dx) > 5) {
      this.isResizing = true;
    }
    if (!this.isResizing) {
      return;
    }
    this.panelEle.style.width = `${this.startWidth + dx}px`;
  }

  updated() {
    this.panelEle = this.shadowRoot!.getElementById('panel')!;
  }

  protected render() {
    return html`
      <div id="container">
        <div
          id="panel"
          style=${styleMap({display: this.expanded ? '' : 'none'})}
        >
          <slot></slot>
        </div>
        <div id="expand-column">
          <div
            id="handle"
            style=${styleMap({'cursor': this.expanded ? 'col-resize' : 'pointer'})}
            @click=${() => {
              if (this.isResizing) {
                this.isResizing = false;
                return;
              }
              this.expanded = !this.expanded;
            }}
            @mousedown=${(e: MouseEvent) => {
              this.startPos = e.x;
              this.startWidth = this.panelEle.clientWidth;
              document.addEventListener('mousemove', this.onResize);
              document.addEventListener('mouseup', () => {
                document.removeEventListener('mousemove', this.onResize);
              }, {once: true});
            }}
          >
            <mwc-icon>drag_handle</mwc-icon>
          </div>
        </div>
    `;
  }

  static styles = css`
    #container {
      display: flex;
      height: 100%;
    }
    #panel {
      overflow-y: hidden;
      height: 100%;
      min-width: var(--min-width, 400px);
      width: var(--width, 400px);
      border-right: 1px solid #DDDDDD;
    }
    #handle {
      user-select: none;
      position: absolute;
      top: 50%;
      transform: translateY(-50%);
      border-left: 13px solid #DDDDDD;
      border-top: 10px solid transparent;
      border-bottom: 10px solid transparent;
      height: 80px;
      width: 0;
    }
    #expand-column {
      height: 100%;
      width: 10px;
      position: relative;
    }
    #handle>mwc-icon {
      position: absolute;
      top: 50%;
      color: grey;
      transform: translateY(-50%) translateX(-19px) rotate(90deg);
    }
  `;
}
