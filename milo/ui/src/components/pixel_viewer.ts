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

import { axisLeft, axisTop, scaleLinear, select as d3Select } from 'd3';
import { css, html, PropertyValues } from 'lit';
import { customElement } from 'lit/decorators.js';
import { autorun, computed, makeObservable, observable, reaction } from 'mobx';

import { consumer, createContextLink } from '../libs/context';
import { MiloBaseElement } from './milo_base';

export interface Coordinate {
  x: number;
  y: number;
}

export const [provideCoord, consumeCoord] = createContextLink<Coordinate>();

/**
 * An element that let users zoom in and view the individual pixel of an image.
 */
@customElement('milo-pixel-viewer')
@consumer
export class PixelViewerElement extends MiloBaseElement {
  @observable.ref label = '';
  @observable.ref imgUrl!: string;
  @consumeCoord() @observable coord: Coordinate = { x: 0, y: 0 };
  @observable.ref pixelSize = 10;

  private canvas = document.createElement('canvas');
  @observable.ref img: HTMLImageElement | null = null;
  @observable.ref private loadedImgUrl = '';
  private ctx = this.canvas.getContext('2d')!;
  private resizeObserver!: ResizeObserver;

  // Prefix 'r' means range. Maps to pixels in the SVG of the pixel viewer.
  @observable.ref private rWidth = 0;
  @observable.ref private rHeight = 0;

  // Prefix 'd' means domain. Maps to pixels in the cropped source image.
  @observable.ref private dWidth = 0;
  @observable.ref private dHeight = 0;
  @observable.ref private dMiddleX = 0;
  @observable.ref private dMiddleY = 0;

  @computed private get coordColor() {
    // The updated image was not loaded yet. Return the default value.
    if (this.loadedImgUrl !== this.img?.src) {
      return [0, 0, 0, 0];
    }
    return this.ctx.getImageData(this.coord.x, this.coord.y, 1, 1).data;
  }
  @computed private get color() {
    const [r, g, b, a] = this.coordColor;
    return `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(2)})`;
  }
  @computed private get labelColor() {
    const [r, g, b, a] = this.coordColor;
    return ((r + g + b) / 3) * (a / 255) > 127 ? 'black' : 'white';
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    const drawImg = () => {
      if (!this.img) {
        return;
      }
      this.ctx.imageSmoothingEnabled = false;
      this.canvas.width = this.img.width;
      this.canvas.height = this.img.height;
      this.ctx.drawImage(this.img, 0, 0, this.img.width, this.img.height);
      this.loadedImgUrl = this.img.src;
    };

    // Load the new image.
    this.addDisposer(
      reaction(
        () => this.imgUrl,
        () => {
          if (this.img) {
            this.img.removeEventListener('load', drawImg);
            this.ctx.clearRect(0, 0, this.img.width, this.img.height);
          }

          this.img = new Image();
          this.img.src = this.imgUrl;
          this.img.addEventListener('load', drawImg, { once: true });
        },
        { fireImmediately: true }
      )
    );
  }

  protected firstUpdated(changeProperties: PropertyValues) {
    super.firstUpdated(changeProperties);

    const svgEle = this.shadowRoot!.querySelector('svg')!;

    // Sync width and height.
    this.resizeObserver = new ResizeObserver(() => {
      const rect = svgEle.getBoundingClientRect();
      this.dWidth = Math.floor(rect.width / this.pixelSize);
      this.dHeight = Math.floor(rect.height / this.pixelSize);
      this.rWidth = this.dWidth * this.pixelSize;
      this.rHeight = this.dHeight * this.pixelSize;
      this.dMiddleX = Math.floor(this.dWidth / 2);
      this.dMiddleY = Math.floor(this.dHeight / 2);
    });
    this.resizeObserver.observe(svgEle);
    this.addDisposer(() => this.resizeObserver.disconnect());

    // Draw zoomed-in image.
    this.addDisposer(
      autorun(() => {
        const xScale = scaleLinear()
          .range([0, this.rWidth])
          .domain([0, this.dWidth]);
        const yScale = scaleLinear()
          .range([0, this.rHeight])
          .domain([0, this.dHeight]);

        // Draw grid.
        const svg = d3Select(svgEle);
        const gridGroup = svg.select('#grid');
        gridGroup.selectChildren().remove();
        const vGridLines = axisTop(xScale)
          .ticks(this.dWidth)
          .tickSize(-this.rHeight)
          .tickFormat(() => '');
        gridGroup.append('g').call(vGridLines);
        const hGridLines = axisLeft(yScale)
          .ticks(this.dHeight)
          .tickSize(-this.rWidth)
          .tickFormat(() => '');
        gridGroup.append('g').call(hGridLines);
      })
    );
  }

  protected render() {
    return html`
      <div id="root">
        <div
          id="label-area"
          style="color: ${this.labelColor}; background-color: ${this.color};"
        >
          ${this.label} (${this.coord.x}, ${this.coord.y}) ${this.color}
        </div>
        <svg
          viewBox="0 0 ${this.rWidth} ${this.rHeight}"
          preserveAspectRatio="xMinYMin slice"
        >
          <g
            transform="
                scale(${this.pixelSize})
                translate(${this.dMiddleX}, ${this.dMiddleY})
                translate(${-this.coord.x}, ${-this.coord.y})
              "
          >
            <image
              href=${this.imgUrl}
              image-rendering="optimizeQuality"
            ></image>
          </g>
          <g id="grid"></g>
          <g id="focus">
            <rect
              x=${this.dMiddleX * this.pixelSize}
              y=${this.dMiddleY * this.pixelSize}
              width=${this.pixelSize}
              height=${this.pixelSize}
            ></rect>
          </g>
        </svg>
      </div>
    `;
  }

  static styles = css`
    :host {
      width: 500px;
      height: 500px;
      overflow: hidden;
    }

    #root {
      display: grid;
      grid-template-rows: auto 1fr;
      grid-gap: 5px;
      width: 100%;
      height: 100%;
    }

    #label-area {
      height: 18px;
      padding: 2px;
      text-align: center;
    }

    svg {
      width: 100%;
      height: 100%;
    }

    image {
      image-rendering: pixelated;
    }

    #grid * {
      stroke: rgba(0, 0, 0, 0.05);
    }

    #focus * {
      stroke: rgba(0, 0, 0, 0.2);
      stroke-width: 3;
      fill: transparent;
    }
  `;
}
