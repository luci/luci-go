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

import { QueryTestHistoryStatsResponseGroup } from '@/common/services/luci_analysis';
import { consumeStore, StoreInstance } from '@/common/store';
import { commonStyles } from '@/common/styles/stylesheets';
import {
  displayDuration,
  parseProtoDurationStr,
} from '@/common/tools/time_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { CELL_PADDING, CELL_SIZE, INNER_CELL_SIZE } from './constants';

@customElement('milo-th-duration-graph')
@consumer
export class TestHistoryDurationGraphElement extends MobxExtLitElement {
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
      const group = this.pageState.statsLoader!.getStats(vHash, i);
      if (!group) {
        ret.push(svg`
          <foreignObject x=${
            CELL_SIZE * i
          } width=${CELL_SIZE} height=${CELL_SIZE}>
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
    const avgDuration = parseProtoDurationStr(group.passedAvgDuration!);
    const avgDurationMs = avgDuration.toMillis();
    this.pageState.setDuration(avgDurationMs);

    return svg`
      <rect
        x=${CELL_PADDING}
        y=${CELL_PADDING}
        width=${INNER_CELL_SIZE}
        height=${INNER_CELL_SIZE}
        fill=${this.pageState.scaleDurationColor(avgDurationMs)}
        style="cursor: pointer;"
        @click=${() => this.pageState.setSelectedGroup(group)}
      >
        <title>Average Duration: ${displayDuration(
          avgDuration,
        )}\nClick to view test run details.</title>
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
      'milo-th-duration-graph': {
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface DurationGraphProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function DurationGraph(props: DurationGraphProps) {
  return <milo-th-duration-graph {...props} class={props.className} />;
}
