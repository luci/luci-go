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

import { AxisScale, axisTop, scaleTime, select as d3Select, timeDay, timeFormat } from 'd3';
import { css, customElement, html } from 'lit-element';
import { computed, observable } from 'mobx';

import { MiloBaseElement } from '../../components/milo_base';
import { consumeTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { consumer } from '../../libs/context';
import { CELL_SIZE, X_AXIS_HEIGHT } from './constants';

@customElement('milo-th-date-axis')
@consumer
export class TestHistoryDateAxisElement extends MiloBaseElement {
  @observable.ref @consumeTestHistoryPageState() pageState!: TestHistoryPageState;

  private readonly dayAdjuster = new ResizeObserver(() => {
    const days = Math.floor((this.getBoundingClientRect().width - 2) / CELL_SIZE);
    this.pageState.days = days;
  });

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
      .domain([this.pageState.now, this.pageState.now.minus({ days: this.pageState.days })])
      .range([0, this.pageState.dates.length * CELL_SIZE]) as AxisScale<Date>;
  }

  @computed private get axisTime() {
    const ret = d3Select(document.createElementNS('http://www.w3.org/2000/svg', 'g'))
      .attr('transform', `translate(1, ${X_AXIS_HEIGHT - 1})`)
      .call(axisTop(this.scaleTime).tickFormat(timeFormat('%Y-%m-%d')).ticks(timeDay.every(1)));

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
    return html`<svg id="x-axis" height=${X_AXIS_HEIGHT}>${this.axisTime}</svg>`;
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
