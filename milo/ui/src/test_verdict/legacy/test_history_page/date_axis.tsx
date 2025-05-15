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

import { Interpolation, Theme } from '@emotion/react';
import {
  AxisScale,
  axisTop,
  scaleTime,
  select as d3Select,
  timeDay,
  timeFormat,
} from 'd3';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';

import { consumeStore, StoreInstance } from '@/common/store';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { CELL_SIZE, X_AXIS_HEIGHT } from './constants';

@customElement('milo-th-date-axis')
@consumer
export class TestHistoryDateAxisElement extends MobxExtLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  private readonly dayAdjuster = new ResizeObserver(() => {
    const days = Math.floor(
      (this.getBoundingClientRect().width - 2) / CELL_SIZE,
    );
    this.pageState.setDays(days);
  });

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.dayAdjuster.observe(this);
  }

  disconnectedCallback() {
    this.dayAdjuster.disconnect();
    super.disconnectedCallback();
  }

  @computed private get scaleTime() {
    return scaleTime()
      .domain([
        this.pageState.latestDate,
        this.pageState.latestDate.minus({ days: this.pageState.days }),
      ])
      .range([0, this.pageState.days * CELL_SIZE]) as AxisScale<Date>;
  }

  @computed private get axisTime() {
    const ret = d3Select(
      document.createElementNS('http://www.w3.org/2000/svg', 'g'),
    )
      .attr('transform', `translate(1, ${X_AXIS_HEIGHT - 1})`)
      .call(
        axisTop(this.scaleTime)
          .tickFormat(timeFormat('%Y-%m-%d'))
          .ticks(timeDay.every(1)),
      );

    ret
      .selectAll('text')
      .attr('y', 0)
      .attr('x', 9)
      .attr('dy', '.35em')
      .attr('transform', 'rotate(-90)')
      .style('text-anchor', 'start');

    return ret;
  }

  protected render() {
    return html`<svg id="x-axis" height=${X_AXIS_HEIGHT}>
      ${this.axisTime}
    </svg>`;
  }

  static styles = css`
    :host {
      display: block;
      height: ${X_AXIS_HEIGHT}px;
    }

    #x-axis {
      width: 100%;
    }
  `;
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-date-axis': {
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface DateAxisProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function DateAxis(props: DateAxisProps) {
  return <milo-th-date-axis {...props} class={props.className} />;
}
