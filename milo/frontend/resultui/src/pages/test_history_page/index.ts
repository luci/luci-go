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

import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { AxisScale, axisTop, scaleTime, select as d3Select, timeFormat } from 'd3';
import { css, customElement, html, svg } from 'lit-element';
import { DateTime } from 'luxon';
import { computed, observable, reaction } from 'mobx';

import '../../components/status_bar';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { VARIANT_STATUS_CLASS_MAP } from '../../libs/constants';
import { consumer, provider } from '../../libs/context';
import { TestHistoryLoader } from '../../models/test_history_loader';
import { NOT_FOUND_URL } from '../../routes';
import { TestVariantStatus } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';

const X_AXIS_HEIGHT = 40;
const INNER_CELL_SIZE = 28;
const CELL_PADDING = 0.5;
const CELL_SIZE = INNER_CELL_SIZE + 2 * CELL_PADDING;

const STATUS_ORDER = [
  TestVariantStatus.EXPECTED,
  TestVariantStatus.FLAKY,
  TestVariantStatus.UNEXPECTEDLY_SKIPPED,
  TestVariantStatus.UNEXPECTED,
];

@customElement('milo-test-history-page')
@provider
@consumer
export class TestHistoryPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;
  @observable.ref private testHistoryLoader: TestHistoryLoader | null = null;
  @observable.ref private realm!: string;
  @observable.ref private testId!: string;

  private readonly now = DateTime.now().startOf('day').plus({ hours: 12 });
  @observable.ref private days = 14;

  @computed private get endDate() {
    return this.now.minus({ days: this.days });
  }

  @computed private get dates() {
    return Array(this.days)
      .fill(0)
      .map((_, i) => this.now.minus({ days: i }));
  }

  @computed private get scaleTime() {
    return scaleTime()
      .domain([this.now, this.now.minus({ days: this.days })])
      .range([0, this.dates.length * CELL_SIZE]) as AxisScale<Date>;
  }

  @computed private get axisTime() {
    const ret = d3Select(document.createElementNS('http://www.w3.org/2000/svg', 'g'))
      .attr('transform', `translate(0, ${X_AXIS_HEIGHT})`)
      .call(axisTop(this.scaleTime).tickFormat(timeFormat('%m-%d')));

    ret
      .selectAll('text')
      .attr('y', 0)
      .attr('x', 9)
      .attr('dy', '.35em')
      .attr('transform', 'rotate(-90)')
      .style('text-anchor', 'start');

    return ret;
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const realm = location.params['realm'];
    const testId = location.params['test_id'];
    if (typeof testId !== 'string' || typeof realm !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.realm = realm;
    this.testId = testId;

    return;
  }

  connectedCallback() {
    super.connectedCallback();

    // Set up TestHistoryLoader.
    this.addDisposer(
      reaction(
        () => [this.realm, this.testId, this.appState?.testHistoryService],
        () => {
          if (!this.realm || !this.testId || !this.appState.testHistoryService) {
            this.testHistoryLoader = null;
            return;
          }

          this.testHistoryLoader = new TestHistoryLoader(
            this.realm,
            this.testId,
            (datetime) => datetime.toFormat('yyyy-MM-dd'),
            this.appState.testHistoryService
          );
        },
        {
          fireImmediately: true,
        }
      )
    );

    // Ensure all the entries are loaded / being loaded.
    this.addDisposer(
      reaction(
        () => [this.testHistoryLoader, this.endDate],
        () => {
          if (!this.testHistoryLoader) {
            return;
          }
          this.testHistoryLoader.loadUntil(this.endDate);
        },
        { fireImmediately: true }
      )
    );
  }

  protected render() {
    return html`
      <table id="test-identifier">
        <tr>
          <td class="id-component-label">Realm</td>
          <td>${this.realm}</td>
        </tr>
        <tr>
          <td class="id-component-label">Test</td>
          <td>${this.testId}</td>
        </tr>
      </table>
      <milo-status-bar .components=${[{ color: 'var(--active-color)', weight: 1 }]}></milo-status-bar>
      <div>
        <svg id="graph" height=${X_AXIS_HEIGHT + CELL_SIZE * (this.testHistoryLoader?.variants.length || 0)}>
          ${this.axisTime}
          <g id="variant-def"></g>
          <g id="main" transform="translate(0, ${X_AXIS_HEIGHT})">
            ${this.testHistoryLoader?.variants.map(
              ([vHash], i) => svg`
              <g transform="translate(0, ${i * CELL_SIZE + CELL_PADDING})">
                ${this.dates.map((d, j) => this.renderEntries(vHash, d, j))}
              </g>
            `
            )}
          </g>
        </svg>
      </div>
    `;
  }

  private renderEntries(vHash: string, date: DateTime, index: number) {
    const entries = this.testHistoryLoader!.getEntries(vHash, date);
    if (entries === null || entries.length === 0) {
      return null;
    }

    const counts = {
      [TestVariantStatus.EXPECTED]: 0,
      [TestVariantStatus.EXONERATED]: 0,
      [TestVariantStatus.FLAKY]: 0,
      [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 0,
      [TestVariantStatus.UNEXPECTED]: 0,
      [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 0,
    };

    for (const entry of entries) {
      counts[entry.status]++;
    }

    let previousHeight = 0;

    return svg`
      <g transform="translate(${index * CELL_SIZE + CELL_PADDING}, 0)">
        ${STATUS_ORDER.map((status) => {
          const height = (INNER_CELL_SIZE * counts[status]) / entries.length;
          const ele = svg`
            <rect
              class="${VARIANT_STATUS_CLASS_MAP[status]}"
              y=${previousHeight}
              width=${INNER_CELL_SIZE}
              height=${height}
            />
          `;
          previousHeight += height;
          return ele;
        })}
        <text
          class="count-label"
          x=${INNER_CELL_SIZE / 2}
          y=${INNER_CELL_SIZE / 2}
        >${entries.length - counts[TestVariantStatus.EXPECTED]}</text>
      </g>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: grid;
        width: 100%;
        grid-template-rows: auto auto 1fr;
      }

      #test-identifier {
        width: 100%;
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
      }
      .id-component-label {
        color: var(--light-text-color);

        /* Shrink the first column */
        width: 0px;
      }

      #graph {
        width: 100%;
      }

      .unexpected {
        fill: var(--failure-color);
      }
      .unexpectedly-skipped {
        fill: var(--critical-failure-color);
      }
      .flaky {
        fill: var(--warning-color);
      }
      .exonerated {
        fill: var(--exonerated-color);
      }
      .expected {
        fill: var(--success-color);
      }

      .count-label {
        fill: white;
        text-anchor: middle;
        alignment-baseline: central;
      }
    `,
  ];
}
