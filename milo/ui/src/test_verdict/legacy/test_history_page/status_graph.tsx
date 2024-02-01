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
import { css, html, svg, SVGTemplateResult } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';

import '@/common/components/status_bar';
import '@/generic_libs/components/dot_spinner';
import './graph_config';
import checkCircle from '@/common/assets/svgs/check_circle_24dp.svg';
import checkCircleStacked from '@/common/assets/svgs/check_circle_stacked_24dp.svg';
import {
  VARIANT_STATUS_CLASS_MAP,
  VERDICT_VARIANT_STATUS_MAP,
} from '@/common/constants/legacy';
import {
  QueryTestHistoryStatsResponseGroup,
  TestVerdictStatus,
} from '@/common/services/luci_analysis';
import { TestVariantStatus } from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { commonStyles } from '@/common/styles/stylesheets';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { CELL_PADDING, CELL_SIZE, INNER_CELL_SIZE } from './constants';

const ICON_PADDING = (CELL_SIZE - 24) / 2;

const STATUS_ORDER = [
  TestVerdictStatus.EXPECTED,
  TestVerdictStatus.EXONERATED,
  TestVerdictStatus.FLAKY,
  TestVerdictStatus.UNEXPECTEDLY_SKIPPED,
  TestVerdictStatus.UNEXPECTED,
];

@customElement('milo-th-status-graph')
@consumer
export class TestHistoryStatusGraphElement extends MobxExtLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    const variants = this.pageState.filteredVariants;
    return html`
      <svg id="graph" height=${CELL_SIZE * variants.length}>
        ${variants.map(
          ([vHash], i) => svg`
            <g transform="translate(1, ${i * CELL_SIZE})">
              <rect
                x="-1"
                height=${CELL_SIZE}
                width=${CELL_SIZE * this.pageState.days + 2}
                fill=${
                  i % 2 === 0 ? 'var(--block-background-color)' : 'transparent'
                }
              />
              ${this.renderRow(vHash)}
            </g>
          `,
        )}
      </svg>
    `;
  }

  private renderRow(vHash: string) {
    const ret: SVGTemplateResult[] = [];

    for (let i = 0; i < this.pageState.days; ++i) {
      const stats = this.pageState.statsLoader!.getStats(vHash, i);
      if (!stats) {
        ret.push(svg`
          <foreignObject x=${
            CELL_SIZE * i
          } width=${CELL_SIZE} height=${CELL_SIZE}>
            <milo-dot-spinner></milo-dot-spinner>
          </foreignObject>
        `);
        break;
      }

      ret.push(svg`
        <g transform="translate(${i * CELL_SIZE}, 0)">
          ${this.renderEntries(stats)}
        </g>
      `);
    }
    return ret;
  }

  private renderEntries(group: QueryTestHistoryStatsResponseGroup) {
    const counts = {
      [TestVerdictStatus.EXPECTED]: group.expectedCount || 0,
      [TestVerdictStatus.EXONERATED]: group.exoneratedCount || 0,
      [TestVerdictStatus.FLAKY]: group.flakyCount || 0,
      [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]:
        group.unexpectedlySkippedCount || 0,
      [TestVerdictStatus.UNEXPECTED]: group.unexpectedCount || 0,
      [TestVerdictStatus.TEST_VERDICT_STATUS_UNSPECIFIED]: 0,
    };
    const totalCount = Object.values(counts).reduce(
      (sum, count) => sum + count,
      0,
    );

    if (totalCount === 0) {
      return;
    }

    const title = svg`<title>Unexpected: ${counts[TestVariantStatus.UNEXPECTED]}
Unexpectedly Skipped: ${counts[TestVariantStatus.UNEXPECTEDLY_SKIPPED]}
Flaky: ${counts[TestVariantStatus.FLAKY]}
Exonerated: ${counts[TestVariantStatus.EXONERATED]}
Expected: ${counts[TestVariantStatus.EXPECTED]}
Click to view test details.</title>`;

    let previousHeight = 0;

    if (counts[TestVariantStatus.EXPECTED] === totalCount) {
      const img = totalCount > 1 ? checkCircleStacked : checkCircle;
      return svg`
        <image
          href=${img}
          x=${ICON_PADDING}
          y=${ICON_PADDING}
          height="24"
          width="24"
          style="cursor: pointer;"
          @click=${() => this.pageState.setSelectedGroup(group)}
        >
          ${title}
        </image>
      `;
    }

    const count =
      (this.pageState.countUnexpected
        ? counts[TestVariantStatus.UNEXPECTED]
        : 0) +
      (this.pageState.countUnexpectedlySkipped
        ? counts[TestVariantStatus.UNEXPECTEDLY_SKIPPED]
        : 0) +
      (this.pageState.countFlaky ? counts[TestVariantStatus.FLAKY] : 0) +
      (this.pageState.countExonerated
        ? counts[TestVariantStatus.EXONERATED]
        : 0);

    const nonEmptyStatusCount = STATUS_ORDER.reduce(
      (c, status) => (counts[status] ? c + 1 : c),
      0,
    );

    return svg`
      ${STATUS_ORDER.map((status) => {
        if (!counts[status]) {
          return;
        }
        // Ensures each non-empty section is at least 1px tall.
        const height =
          ((INNER_CELL_SIZE - nonEmptyStatusCount) * counts[status]) /
            totalCount +
          1;
        const ele = svg`
          <rect
            class="${
              VARIANT_STATUS_CLASS_MAP[VERDICT_VARIANT_STATUS_MAP[status]]
            }"
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
        x=${CELL_SIZE / 2 + CELL_PADDING}
        y=${CELL_SIZE / 2 + CELL_PADDING}
      >${count}</text>
      <rect
        width=${INNER_CELL_SIZE + CELL_PADDING}
        height=${INNER_CELL_SIZE + CELL_PADDING}
        fill="transparent"
        style="cursor: pointer;"
        @click=${() => this.pageState.setSelectedGroup(group)}
      >
        ${title}
      </rect>
    `;
  }

  static styles = [
    commonStyles,
    css`
      :host {
        display: block;
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

      milo-dot-spinner {
        color: var(--active-color);
        font-size: 12px;
        line-height: ${CELL_SIZE}px;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-status-graph': {
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface StatusGraphProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function StatusGraph(props: StatusGraphProps) {
  return <milo-th-status-graph {...props} class={props.className} />;
}
