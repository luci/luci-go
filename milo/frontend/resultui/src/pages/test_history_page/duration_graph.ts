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

import { css, customElement, html, svg, SVGTemplateResult } from 'lit-element';
import { Duration } from 'luxon';
import { makeObservable, observable } from 'mobx';

import { MiloBaseElement } from '../../components/milo_base';
import { consumeTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { consumer } from '../../libs/context';
import { displayDuration, parseProtoDuration } from '../../libs/time_utils';
import { QueryTestHistoryStatsResponseGroup } from '../../services/weetbix';
import commonStyle from '../../styles/common_style.css';
import { CELL_PADDING, CELL_SIZE, INNER_CELL_SIZE } from './constants';

@customElement('milo-th-duration-graph')
@consumer
export class TestHistoryDurationGraphElement extends MiloBaseElement {
  @observable.ref @consumeTestHistoryPageState() pageState!: TestHistoryPageState;

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
                fill=${i % 2 === 0 ? 'var(--block-background-color)' : 'transparent'}
              />
              ${this.renderRow(vHash)}
            </g>
          `
        )}
      </svg>
    `;
  }

  private renderRow(vHash: string) {
    const ret: SVGTemplateResult[] = [];

    for (let i = 0; i < this.pageState.days; ++i) {
      const group = this.pageState.statsLoader!.getStats(vHash, i);
      if (!group) {
        ret.push(svg`
          <foreignObject x=${CELL_SIZE * i} width=${CELL_SIZE} height=${CELL_SIZE}>
            <milo-dot-spinner></milo-dot-spinner>
          </foreignObject>
        `);
        break;
      }

      if (!group?.passedAvgDuration) {
        continue;
      }

      ret.push(svg`
        <g transform="translate(${i * CELL_SIZE}, 0)">
          ${this.renderEntries(group)}
        </g>
      `);
    }
    return ret;
  }

  private renderEntries(group: QueryTestHistoryStatsResponseGroup) {
    const averageDurationMs = parseProtoDuration(group.passedAvgDuration!);
    if (!this.pageState.durationInitialized) {
      this.pageState.maxDurationMs = averageDurationMs;
      this.pageState.minDurationMs = averageDurationMs;
      this.pageState.durationInitialized = true;
    } else {
      if (averageDurationMs > this.pageState.maxDurationMs) {
        this.pageState.maxDurationMs = averageDurationMs;
      }
      if (averageDurationMs < this.pageState.minDurationMs) {
        this.pageState.minDurationMs = averageDurationMs;
      }
    }

    return svg`
      <rect
        x=${CELL_PADDING}
        y=${CELL_PADDING}
        width=${INNER_CELL_SIZE}
        height=${INNER_CELL_SIZE}
        fill=${this.pageState.scaleDurationColor(averageDurationMs)}
        style="cursor: pointer;"
        @click=${() => {
          this.pageState.selectedGroup = group;
        }}
      >
        <title>Average Duration: ${displayDuration(
          Duration.fromMillis(averageDurationMs)
        )}\nClick to view test run details.</title>
      </rect>
    `;
  }

  static styles = [
    commonStyle,
    css`
      #graph {
        width: 100%;
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
