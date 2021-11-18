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

import { css, customElement, html, svg } from 'lit-element';
import { DateTime } from 'luxon';
import { observable } from 'mobx';

import './graph_config';
import '../../components/status_bar';
import { MiloBaseElement } from '../../components/milo_base';
import { consumeTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { VARIANT_STATUS_CLASS_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { TestVariantStatus } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';
import { CELL_PADDING, CELL_SIZE, INNER_CELL_SIZE } from './constants';

const ICON_PADDING = (CELL_SIZE - 24) / 2;

const STATUS_ORDER = [
  TestVariantStatus.EXPECTED,
  TestVariantStatus.FLAKY,
  TestVariantStatus.UNEXPECTEDLY_SKIPPED,
  TestVariantStatus.UNEXPECTED,
];

@customElement('milo-th-status-graph')
@consumer
export class TestHistoryStatusGraphElement extends MiloBaseElement {
  @observable.ref @consumeTestHistoryPageState() pageState!: TestHistoryPageState;

  protected render() {
    const variants = this.pageState.testHistoryLoader!.variants;
    return html`
      <svg id="graph" height=${CELL_SIZE * variants.length}>
        ${variants.map(
          ([vHash], i) => svg`
            <g transform="translate(1, ${i * CELL_SIZE})">
              <rect
                x="-1"
                height=${CELL_SIZE}
                width=${CELL_SIZE * this.pageState.days + 2}
                fill=${i % 2 === 0 ? 'var(--block-background-color)' : 'transparent'}
              />
              ${this.pageState.dates.map((d, j) => this.renderEntries(vHash, d, j))}
            </g>
          `
        )}
      </svg>
    `;
  }

  private renderEntries(vHash: string, date: DateTime, index: number) {
    const entries = this.pageState.testHistoryLoader!.getEntries(vHash, date);
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

    if (counts[TestVariantStatus.EXPECTED] === entries.length) {
      const img =
        entries.length > 1
          ? '/ui/immutable/svgs/check_circle_stacked_24dp.svg'
          : '/ui/immutable/svgs/check_circle_24dp.svg';
      return svg`
        <g transform="translate(${index * CELL_SIZE + ICON_PADDING}, 0)">
          <image href=${img} y=${ICON_PADDING} height="24" width="24" />
        </g>
      `;
    }

    const count =
      (this.pageState.countUnexpected ? counts[TestVariantStatus.UNEXPECTED] : 0) +
      (this.pageState.countUnexpectedlySkipped ? counts[TestVariantStatus.UNEXPECTEDLY_SKIPPED] : 0) +
      (this.pageState.countFlaky ? counts[TestVariantStatus.FLAKY] : 0);

    return svg`
      <g transform="translate(${index * CELL_SIZE + CELL_PADDING}, ${CELL_PADDING})">
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
          x=${CELL_SIZE / 2}
          y=${CELL_SIZE / 2}
        >${count}</text>
      </g>
    `;
  }

  static styles = [
    commonStyle,
    css`
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
