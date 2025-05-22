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
import nextPlan from '@/common/assets/svgs/next_plan_24dp.svg';
import nextPlanStacked from '@/common/assets/svgs/next_plan_stacked_24dp.svg';
import { VERDICT_STATUS_CLASS_MAP } from '@/common/constants/legacy';
import {
  QueryTestHistoryStatsResponseGroup,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/luci_analysis';
import { consumeStore, StoreInstance } from '@/common/store';
import { GraphType } from '@/common/store/test_history_page';
import { commonStyles } from '@/common/styles/stylesheets';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { CELL_PADDING, CELL_SIZE, INNER_CELL_SIZE } from './constants';

const ICON_PADDING = (CELL_SIZE - 24) / 2;

const STATUS_ORDER: (
  | TestVerdict_Status
  | TestVerdict_StatusOverride.EXONERATED
)[] = [
  TestVerdict_Status.PASSED,
  TestVerdict_StatusOverride.EXONERATED,
  TestVerdict_Status.SKIPPED,
  TestVerdict_Status.FLAKY,
  TestVerdict_Status.PRECLUDED,
  TestVerdict_Status.EXECUTION_ERRORED,
  TestVerdict_Status.FAILED,
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
    const showExonerations =
      this.pageState.graphType === GraphType.STATUS_WITH_EXONERATION;

    const counts = {
      [TestVerdict_Status.FAILED]: group.verdictCounts.failed || 0,
      [TestVerdict_Status.EXECUTION_ERRORED]:
        group.verdictCounts.executionErrored || 0,
      [TestVerdict_Status.PRECLUDED]: group.verdictCounts.precluded || 0,
      [TestVerdict_Status.FLAKY]: group.verdictCounts.flaky || 0,
      [TestVerdict_Status.SKIPPED]: group.verdictCounts.skipped || 0,
      [TestVerdict_Status.PASSED]: group.verdictCounts.passed || 0,
      [TestVerdict_StatusOverride.EXONERATED]: 0,
      [TestVerdict_Status.STATUS_UNSPECIFIED]: 0,
    };
    if (showExonerations) {
      counts[TestVerdict_StatusOverride.EXONERATED] =
        (group.verdictCounts.failedExonerated || 0) +
        (group.verdictCounts.executionErroredExonerated || 0) +
        (group.verdictCounts.precludedExonerated || 0);

      counts[TestVerdict_Status.FAILED] =
        counts[TestVerdict_Status.FAILED] -
        (group.verdictCounts.failedExonerated || 0);
      counts[TestVerdict_Status.EXECUTION_ERRORED] =
        counts[TestVerdict_Status.EXECUTION_ERRORED] -
        (group.verdictCounts.executionErroredExonerated || 0);
      counts[TestVerdict_Status.PRECLUDED] =
        counts[TestVerdict_Status.PRECLUDED] -
        (group.verdictCounts.precludedExonerated || 0);
    }

    const totalCount = Object.values(counts).reduce(
      (sum, count) => sum + count,
      0,
    );

    if (totalCount === 0) {
      return;
    }

    const title = svg`<title>Failed: ${counts[TestVerdict_Status.FAILED]}
Execution Errored: ${counts[TestVerdict_Status.EXECUTION_ERRORED]}
Precluded: ${counts[TestVerdict_Status.PRECLUDED]}
Flaky: ${counts[TestVerdict_Status.FLAKY]}
Skipped: ${counts[TestVerdict_Status.SKIPPED]}
${showExonerations ? `Exonerated: ${counts[TestVerdict_StatusOverride.EXONERATED]}\n` : ''}Passed: ${counts[TestVerdict_Status.PASSED]}
Click to view test details.</title>`;

    let previousHeight = 0;

    if (
      counts[TestVerdict_Status.PASSED] === totalCount &&
      !this.pageState.countPassed
    ) {
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
    if (
      counts[TestVerdict_Status.SKIPPED] === totalCount &&
      !this.pageState.countSkipped
    ) {
      const img = totalCount > 1 ? nextPlanStacked : nextPlan;
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
      (this.pageState.countFailed ? counts[TestVerdict_Status.FAILED] : 0) +
      (this.pageState.countExecutionErrored
        ? counts[TestVerdict_Status.EXECUTION_ERRORED]
        : 0) +
      (this.pageState.countPrecluded
        ? counts[TestVerdict_Status.PRECLUDED]
        : 0) +
      (this.pageState.countFlaky ? counts[TestVerdict_Status.FLAKY] : 0) +
      (this.pageState.countSkipped ? counts[TestVerdict_Status.SKIPPED] : 0) +
      (this.pageState.countPassed ? counts[TestVerdict_Status.PASSED] : 0) +
      (this.pageState.countExonerated
        ? counts[TestVerdict_StatusOverride.EXONERATED]
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
            class="${VERDICT_STATUS_CLASS_MAP[status]}"
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

      .failed-verdict {
        fill: var(--failure-color);
      }
      .execution-errored-verdict {
        fill: var(--critical-failure-color);
      }
      .precluded-verdict {
        fill: var(--precluded-color);
      }
      .flaky-verdict {
        fill: var(--warning-color);
      }
      .skipped-verdict {
        fill: var(--skipped-color);
      }
      .passed-verdict {
        fill: var(--success-color);
      }
      .exonerated-verdict {
        fill: var(--exonerated-color);
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

declare module 'react' {
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
