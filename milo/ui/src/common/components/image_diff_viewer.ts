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

import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable, reaction } from 'mobx';

import '@/generic_libs/components/hotkey';
import '@/generic_libs/components/pixel_viewer';
import { Artifact } from '@/common/services/resultdb';
import { commonStyles } from '@/common/styles/stylesheets';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import {
  Coordinate,
  provideCoord,
} from '@/generic_libs/components/pixel_viewer';
import { provider } from '@/generic_libs/tools/lit_context';

const enum ViewOption {
  Expected,
  Actual,
  Diff,
  Animated,
  SideBySide,
}

const VIEW_OPTION_CLASS_MAP = Object.freeze({
  [ViewOption.Expected]: 'expected',
  [ViewOption.Actual]: 'actual',
  [ViewOption.Diff]: 'diff',
  [ViewOption.Animated]: 'animated',
  [ViewOption.SideBySide]: 'side-by-side',
});

/**
 * Renders an image diff artifact set, including expected image, actual image
 * and image diff.
 */
// TODO(weiweilin): improve error handling.
@customElement('milo-image-diff-viewer')
@provider
export class ImageDiffViewerElement extends MobxExtLitElement {
  @observable.ref expected!: Artifact;
  @observable.ref actual!: Artifact;
  @observable.ref diff!: Artifact;

  @observable.ref private showPixelViewers = false;
  @observable.ref @provideCoord() coord: Coordinate = { x: 0, y: 0 };

  @computed private get expectedImgUrl() {
    return getRawArtifactURLPath(this.expected.name);
  }
  @computed private get actualImgUrl() {
    return getRawArtifactURLPath(this.actual.name);
  }
  @computed private get diffImgUrl() {
    return getRawArtifactURLPath(this.diff.name);
  }
  @observable.ref private viewOption = ViewOption.Animated;

  private readonly updateCoord = (e: MouseEvent) => {
    if (!this.showPixelViewers) {
      return;
    }

    const rect = (e.target as HTMLImageElement).getBoundingClientRect();
    const x = Math.max(Math.round(e.clientX - rect.left), 0);
    const y = Math.max(Math.round(e.clientY - rect.top), 0);
    this.coord = { x, y };
  };

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    const hidePixelViewers = () => (this.showPixelViewers = false);
    window.addEventListener('click', hidePixelViewers);
    this.addDisposer(() =>
      window.removeEventListener('click', hidePixelViewers),
    );
    this.addDisposer(
      reaction(
        () => this.coord,
        (coord) => {
          // Emulate @property() update.
          this.updated(new Map([['coord', coord]]));
        },
        { fireImmediately: true },
      ),
    );
  }

  protected render() {
    return html`
      <div
        id="pixel-viewers"
        style=${styleMap({ display: this.showPixelViewers ? '' : 'none' })}
      >
        <milo-hotkey
          id="close-viewers-instruction"
          .key=${'esc'}
          .handler=${() => (this.showPixelViewers = false)}
          @click=${() => (this.showPixelViewers = false)}
        >
          Click again or press ESC to close the pixel viewers.
        </milo-hotkey>
        <div id="pixel-viewer-grid">
          <milo-pixel-viewer
            .label=${'expected:'}
            .imgUrl=${this.expectedImgUrl}
          ></milo-pixel-viewer>
          <milo-pixel-viewer
            .label=${'actual:'}
            .imgUrl=${this.actualImgUrl}
          ></milo-pixel-viewer>
          <milo-pixel-viewer
            .label=${'diff:'}
            .imgUrl=${this.diffImgUrl}
          ></milo-pixel-viewer>
        </div>
      </div>
      <div id="options">
        <input
          type="radio"
          name="view-option"
          id="expected"
          @change=${() => (this.viewOption = ViewOption.Expected)}
          ?checked=${this.viewOption === ViewOption.Expected}
        />
        <label for="expected">Expected</label>
        <input
          type="radio"
          name="view-option"
          id="actual"
          @change=${() => (this.viewOption = ViewOption.Actual)}
          ?checked=${this.viewOption === ViewOption.Actual}
        />
        <label for="actual">Actual</label>
        <input
          type="radio"
          name="view-option"
          id="diff"
          @change=${() => (this.viewOption = ViewOption.Diff)}
          ?checked=${this.viewOption === ViewOption.Diff}
        />
        <label for="diff">Diff</label>
        <input
          type="radio"
          name="view-option"
          id="animated"
          @change=${() => (this.viewOption = ViewOption.Animated)}
          ?checked=${this.viewOption === ViewOption.Animated}
        />
        <label for="animated">Animated</label>
        <input
          type="radio"
          name="view-option"
          id="side-by-side"
          @change=${() => (this.viewOption = ViewOption.SideBySide)}
          ?checked=${this.viewOption === ViewOption.SideBySide}
        />
        <label for="side-by-side">Side by side</label>
      </div>
      <div id="content" class=${VIEW_OPTION_CLASS_MAP[this.viewOption]}>
        ${this.renderImage('expected-image', 'expected', this.expectedImgUrl)}
        ${this.renderImage('actual-image', 'actual', this.actualImgUrl)}
        ${this.renderImage('diff-image', 'diff', this.diffImgUrl)}
      </div>
    `;
  }

  private renderImage(id: string, label: string, url: string) {
    return html`
      <div id=${id} class="image">
        <div>
          ${label} (view raw <a href=${url} target="_blank">here</a> or click on
          the image to zoom in.)
        </div>
        <img
          src=${url}
          @mousemove=${this.updateCoord}
          @click=${(e: MouseEvent) => {
            e.stopPropagation();
            this.showPixelViewers = !this.showPixelViewers;
            this.updateCoord(e);
          }}
        />
      </div>
    `;
  }

  static styles = [
    commonStyles,
    css`
      :host {
        display: block;
        overflow: hidden;
      }

      #pixel-viewers {
        width: 100%;
        position: fixed;
        left: 0;
        top: 0;
        z-index: 999;
        background-color: var(--dark-background-color);
      }
      #close-viewers-instruction {
        color: white;
        padding: 5px;
      }
      #pixel-viewer-grid {
        display: grid;
        grid-template-columns: 1fr 1fr 1fr;
        grid-gap: 5px;
        padding: 5px;
        height: 300px;
        width: 100%;
      }
      #pixel-viewer-grid > * {
        width: 100%;
        height: 100%;
      }

      #options {
        margin: 5px;
      }
      #options > label {
        margin-right: 5px;
      }
      .raw-link:not(:last-child):after {
        content: ',';
      }

      #content {
        white-space: nowrap;
        overflow-x: auto;
        margin: 15px;
        position: relative;
        top: 0;
        left: 0;
      }
      .image {
        display: none;
      }

      .expected #expected-image {
        display: block;
      }
      .actual #actual-image {
        display: block;
      }
      .diff #diff-image {
        display: block;
      }

      .animated .image {
        animation-name: blink;
        animation-duration: 2s;
        animation-timing-function: steps(1);
        animation-iteration-count: infinite;
      }
      .animated #expected-image {
        display: block;
        position: absolute;
        animation-delay: -1s;
      }
      .animated #actual-image {
        display: block;
        position: static;
        animation-direction: normal;
      }
      @keyframes blink {
        0% {
          opacity: 1;
        }
        50% {
          opacity: 0;
        }
      }

      .side-by-side .image {
        display: inline-block;
      }
    `,
  ];
}
