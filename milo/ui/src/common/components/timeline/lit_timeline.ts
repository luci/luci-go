// Copyright 2022 The LUCI Authors.
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

import { MobxLitElement } from '@adobe/lit-mobx';
import {
  axisBottom,
  axisLeft,
  axisTop,
  BaseType,
  scaleLinear,
  scaleTime,
  select as d3Select,
  Selection,
  timeMillisecond,
} from 'd3';
import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { DateTime } from 'luxon';
import { makeObservable, observable } from 'mobx';

import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
} from '@/common/components/tooltip';
import { PREDEFINED_TIME_INTERVALS } from '@/common/constants/time';
import { commonStyles } from '@/common/styles/stylesheets';
import {
  displayDuration,
  NUMERIC_TIME_FORMAT,
} from '@/common/tools/time_utils';
import { enumerate } from '@/generic_libs/tools/iter_utils';
import { roundDown } from '@/generic_libs/tools/utils';

const TOP_AXIS_HEIGHT = 35;
const BOTTOM_AXIS_HEIGHT = 25;
const BORDER_SIZE = 1;
const HALF_BORDER_SIZE = BORDER_SIZE / 2;

const ROW_HEIGHT = 30;
const BLOCK_HEIGHT = 24;
const BLOCK_MARGIN = (ROW_HEIGHT - BLOCK_HEIGHT) / 2 - HALF_BORDER_SIZE;
const BLOCK_EXTRA_WIDTH = 2;

const TEXT_HEIGHT = 10;
const INV_TEXT_OFFSET = ROW_HEIGHT / 2 + TEXT_HEIGHT / 2;
const TEXT_MARGIN = 10;

const SIDE_PANEL_WIDTH = 400;
const SIDE_PANEL_RECT_WIDTH =
  SIDE_PANEL_WIDTH - BLOCK_MARGIN * 2 - BORDER_SIZE * 2;
const MIN_GRAPH_WIDTH = 500 + SIDE_PANEL_WIDTH;

const LIST_ITEM_WIDTH = SIDE_PANEL_RECT_WIDTH - TEXT_MARGIN * 2;
const LIST_ITEM_HEIGHT = 16;
const LIST_ITEM_X_OFFSET = BLOCK_MARGIN + TEXT_MARGIN + BORDER_SIZE;
const LIST_ITEM_Y_OFFSET = BLOCK_MARGIN + (BLOCK_HEIGHT - LIST_ITEM_HEIGHT) / 2;

const V_GRID_LINE_MAX_GAP = 80;

export interface TimelineBlock {
  start?: DateTime;
  end?: DateTime;
  text: string;
  href?: string;
}

@customElement('milo-timeline')
export class TimelineElement extends MobxLitElement {
  // The time to start the timeline.
  // Blocks that are not between startTime and endTime will be rendered as a blank row.
  @observable.ref startTime = DateTime.now();
  // Label for the start time rendered in the timeline header.
  @observable.ref startTimeLabel = 'Start Time';
  // The time to end the timeline.
  // Blocks that are not between startTime and endTime will be rendered as a blank row.
  @observable.ref endTime = DateTime.now();
  // Label for the end time rendered in the timeline header.
  @observable.ref endTimeLabel = 'End Time';
  // The blocks of time to render on the timeline.
  @observable.ref blocks: TimelineBlock[] = [];

  @observable.ref private width = MIN_GRAPH_WIDTH;

  private headerEle!: HTMLDivElement;
  private footerEle!: HTMLDivElement;
  private sidePanelEle!: HTMLDivElement;
  private bodyEle!: HTMLDivElement;
  private relativeTimeText!: Selection<
    SVGTextElement,
    unknown,
    null,
    undefined
  >;

  // Properties shared between render methods.
  private bodyHeight!: number;
  private scaleTime!: d3.ScaleTime<number, number, never>;
  private scaleStep!: d3.ScaleLinear<number, number, never>;
  private timeInterval!: d3.TimeInterval;
  // prevTimeout is the id of the timeout set in the previous render call.  If it is null there is no pending timeout.
  private prevTimeout: number | null = null;

  constructor() {
    super();
    makeObservable(this);
  }

  render() {
    const startTime = this.startTime.toMillis();
    const endTime = this.endTime.toMillis();
    this.bodyHeight = this.blocks.length * ROW_HEIGHT - BORDER_SIZE;
    const bodyWidth = this.width - SIDE_PANEL_WIDTH;
    const padding =
      Math.ceil(((endTime - startTime) * BLOCK_EXTRA_WIDTH) / bodyWidth) / 2;

    // Calc attributes shared among components.
    this.scaleTime = scaleTime()
      // Add a bit of padding to ensure everything renders in the viewport.
      .domain([startTime - padding, endTime + padding])
      // Ensure the right border is rendered within the viewport, while the left
      // border overlaps with the right border of the side-panel.
      .range([-HALF_BORDER_SIZE, bodyWidth - HALF_BORDER_SIZE]);
    this.scaleStep = scaleLinear()
      .domain([0, this.blocks.length])
      // Ensure the top and bottom borders are not rendered.
      .range([-HALF_BORDER_SIZE, this.bodyHeight + HALF_BORDER_SIZE]);

    const maxInterval =
      (endTime - startTime + 2 * padding) / (bodyWidth / V_GRID_LINE_MAX_GAP);

    this.timeInterval = timeMillisecond.every(
      roundDown(maxInterval, PREDEFINED_TIME_INTERVALS),
    )!;

    // Render each component.
    this.renderHeader();
    this.renderFooter();
    this.renderSidePanel();
    this.renderBody();

    return html`<div id="timeline">
      ${this.sidePanelEle}${this.headerEle}${this.bodyEle}${this.footerEle}
    </div>`;
  }

  private renderHeader() {
    this.headerEle = document.createElement('div');
    const svg = d3Select(this.headerEle)
      .attr('id', 'header')
      .append('svg')
      .attr('viewport', `0 0 ${this.width} ${TOP_AXIS_HEIGHT}`);

    if (this.startTimeLabel) {
      svg
        .append('text')
        .attr('x', TEXT_MARGIN)
        .attr('y', TOP_AXIS_HEIGHT - TEXT_MARGIN / 2)
        .attr('font-weight', '500')
        .text(
          `${this.startTimeLabel}: ${this.startTime.toFormat(
            NUMERIC_TIME_FORMAT,
          )}`,
        );
    }

    const headerRootGroup = svg
      .append('g')
      .attr(
        'transform',
        `translate(${SIDE_PANEL_WIDTH}, ${TOP_AXIS_HEIGHT - HALF_BORDER_SIZE})`,
      );
    const topAxis = axisTop(this.scaleTime).ticks(this.timeInterval);
    headerRootGroup.call(topAxis);

    this.relativeTimeText = headerRootGroup
      .append('text')
      .style('opacity', 0)
      .attr('id', 'relative-time')
      .attr('fill', 'red')
      .attr('y', -TEXT_HEIGHT - TEXT_MARGIN)
      .attr('text-anchor', 'end');

    // Top border for the side panel.
    headerRootGroup
      .append('line')
      .attr('x1', -SIDE_PANEL_WIDTH)
      .attr('stroke', 'var(--default-text-color)');
  }

  private renderFooter() {
    this.footerEle = document.createElement('div');
    const svg = d3Select(this.footerEle)
      .attr('id', 'footer')
      .append('svg')
      .attr('viewport', `0 0 ${this.width} ${BOTTOM_AXIS_HEIGHT}`);

    if (this.endTime) {
      svg
        .append('text')
        .attr('x', TEXT_MARGIN)
        .attr('y', TEXT_HEIGHT + TEXT_MARGIN / 2)
        .attr('font-weight', '500')
        .text(
          `${this.endTimeLabel}: ${this.endTime.toFormat(NUMERIC_TIME_FORMAT)}`,
        );
    }

    const footerRootGroup = svg
      .append('g')
      .attr('transform', `translate(${SIDE_PANEL_WIDTH}, ${HALF_BORDER_SIZE})`);
    const bottomAxis = axisBottom(this.scaleTime).ticks(this.timeInterval);
    footerRootGroup.call(bottomAxis);

    // Bottom border for the side panel.
    footerRootGroup
      .append('line')
      .attr('x1', -SIDE_PANEL_WIDTH)
      .attr('stroke', 'var(--default-text-color)');
  }

  private renderSidePanel() {
    this.sidePanelEle = document.createElement('div');
    const svg = d3Select(this.sidePanelEle)
      .style('width', SIDE_PANEL_WIDTH + 'px')
      .style('height', this.bodyHeight + 'px')
      .attr('id', 'side-panel')
      .append('svg')
      .attr('viewport', `0 0 ${SIDE_PANEL_WIDTH} ${this.bodyHeight}`);

    // Grid lines
    const horizontalGridLines = axisLeft(this.scaleStep)
      .ticks(this.blocks.length)
      .tickFormat(() => '')
      .tickSize(-SIDE_PANEL_WIDTH)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(horizontalGridLines);

    for (const [i, block] of enumerate(this.blocks)) {
      const blockGroup = svg
        .append('g')
        .attr('class', block.end ? 'started' : 'success')
        .attr('transform', `translate(0, ${i * ROW_HEIGHT})`);

      const rect = blockGroup
        .append('rect')
        .attr('x', BLOCK_MARGIN + BORDER_SIZE)
        .attr('y', BLOCK_MARGIN)
        .attr('width', SIDE_PANEL_RECT_WIDTH)
        .attr('height', BLOCK_HEIGHT);
      this.installBlockInteractionHandlers(rect, block);

      const listItem = blockGroup
        .append('foreignObject')
        .attr('class', 'not-intractable')
        .attr('x', LIST_ITEM_X_OFFSET)
        .attr('y', LIST_ITEM_Y_OFFSET)
        .attr('height', BLOCK_HEIGHT - LIST_ITEM_Y_OFFSET)
        .attr('width', LIST_ITEM_WIDTH);
      const blockText = listItem.append('xhtml:span').text(block.text);

      blockText.attr('class', (block.href ? 'hyperlink' : '') + ' nowrap');
    }

    // Left border.
    svg
      .append('line')
      .attr('x1', HALF_BORDER_SIZE)
      .attr('x2', HALF_BORDER_SIZE)
      .attr('y2', this.bodyHeight)
      .attr('stroke', 'var(--default-text-color)');
    // Right border.
    svg
      .append('line')
      .attr('x1', SIDE_PANEL_WIDTH - HALF_BORDER_SIZE)
      .attr('x2', SIDE_PANEL_WIDTH - HALF_BORDER_SIZE)
      .attr('y2', this.bodyHeight)
      .attr('stroke', 'var(--default-text-color)');
  }

  private renderBody() {
    const bodyWidth = this.width - SIDE_PANEL_WIDTH;
    this.bodyEle = document.createElement('div');
    const svg = d3Select(this.bodyEle)
      .attr('id', 'body')
      .style('width', bodyWidth + 'px')
      .style('height', this.bodyHeight + 'px')
      .append('svg')
      .attr('viewport', `0 0 ${bodyWidth} ${this.bodyHeight}`);

    // Grid lines
    const verticalGridLines = axisTop(this.scaleTime)
      .ticks(this.timeInterval)
      .tickSize(-this.bodyHeight)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(verticalGridLines);
    const horizontalGridLines = axisLeft(this.scaleStep)
      .ticks(this.blocks.length)
      .tickFormat(() => '')
      .tickSize(-bodyWidth)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(horizontalGridLines);

    for (const [i, block] of enumerate(this.blocks)) {
      const start = this.scaleTime(
        block.start?.toMillis() || this.endTime.toMillis(),
      );
      const end = this.scaleTime(
        block.end?.toMillis() || this.endTime.toMillis(),
      );

      const blockGroup = svg
        .append('g')
        .attr('class', block.end ? 'started' : 'success')
        .attr('transform', `translate(${start}, ${i * ROW_HEIGHT})`);

      // Add extra width so tiny blocks are visible.
      const width = end - start + BLOCK_EXTRA_WIDTH;

      blockGroup
        .append('rect')
        .attr('x', -BLOCK_EXTRA_WIDTH / 2)
        .attr('y', BLOCK_MARGIN)
        .attr('width', width)
        .attr('height', BLOCK_HEIGHT);

      const blockText = blockGroup
        .append('text')
        .attr('text-anchor', 'start')
        .attr('x', TEXT_MARGIN)
        .attr('y', INV_TEXT_OFFSET)
        .text(block.text);

      // Wail until the next event cycle so blockText is rendered when we call
      // this.getBBox();

      // Cancel timeouts for previous renders so we don't do useless page layouts.
      if (this.prevTimeout) {
        clearTimeout(this.prevTimeout);
      }
      this.prevTimeout = window.setTimeout(() => {
        this.prevTimeout = null;
        // eslint-disable-next-line @typescript-eslint/no-this-alias
        const self = this;
        blockText.each(function () {
          // This is the standard d3 API.
          // eslint-disable-next-line no-invalid-this
          const textBBox = this.getBBox();
          const x1 = Math.min(textBBox.x, -BLOCK_EXTRA_WIDTH / 2);
          const x2 = Math.max(
            textBBox.x + textBBox.width,
            BLOCK_MARGIN + width,
          );

          // This makes the inv text easier to interact with.
          const eventTargetRect = blockGroup
            .append('rect')
            .attr('x', x1)
            .attr('y', BLOCK_MARGIN)
            .attr('width', x2 - x1)
            .attr('height', BLOCK_HEIGHT)
            .attr('class', 'invisible');

          self.installBlockInteractionHandlers(eventTargetRect, block);
        });
      }, 10);
    }

    const yRuler = svg
      .append('line')
      .style('opacity', 0)
      .attr('stroke', 'red')
      .attr('pointer-events', 'none')
      .attr('y1', 0)
      .attr('y2', this.bodyHeight);

    let svgBox: DOMRect | null = null;
    svg.on('mouseover', () => {
      this.relativeTimeText.style('opacity', 1);
      yRuler.style('opacity', 1);
    });
    svg.on('mouseout', () => {
      this.relativeTimeText.style('opacity', 0);
      yRuler.style('opacity', 0);
    });
    svg.on('mousemove', (e: MouseEvent) => {
      if (svgBox === null) {
        svgBox = svg.node()!.getBoundingClientRect();
      }
      const x = e.pageX - svgBox.x;

      yRuler.attr('x1', x);
      yRuler.attr('x2', x);

      const time = DateTime.fromJSDate(this.scaleTime.invert(x));
      const duration = time.diff(this.startTime!);
      this.relativeTimeText.attr('x', x);
      this.relativeTimeText.text(displayDuration(duration) + ' since start');
    });

    // Right border.
    svg
      .append('line')
      .attr('x1', bodyWidth - HALF_BORDER_SIZE)
      .attr('x2', bodyWidth - HALF_BORDER_SIZE)
      .attr('y2', this.bodyHeight)
      .attr('stroke', 'var(--default-text-color)');
  }

  /**
   * Installs handlers for interacting with a inv object.
   */
  private installBlockInteractionHandlers<T extends BaseType>(
    ele: Selection<T, unknown, null, undefined>,
    block: TimelineBlock,
  ) {
    if (block.href) {
      ele
        .attr('class', ele.attr('class') + ' clickable')
        .on('click', (e: MouseEvent) => {
          e.stopPropagation();
          window.open(block.href, '_blank');
        });
    }

    ele
      .on('mouseover', (e: MouseEvent) => {
        const tooltip = document.createElement('div');
        render(
          html`
                <table>
                  <tr>
                    <td colspan="2">Click to open invocation.</td>
                  </tr>
                  <tr>
                    <td>Started:</td>
                    <td>
                      ${(block.start || this.endTime).toFormat(
                        NUMERIC_TIME_FORMAT,
                      )}
                      (after ${displayDuration(
                        (block.start || this.endTime).diff(this.startTime),
                      )})
                    </td>
                  </tr>
                  <tr>
                    <td>Ended:</td>
                    <td>
                      ${
                        block.end
                          ? block.end.toFormat(NUMERIC_TIME_FORMAT) +
                            ` (after ${displayDuration(
                              block.end.diff(this.startTime),
                            )})`
                          : 'N/A'
                      }</td>
                  </tr>
                  <tr>
                    <td>Duration:</td>
                    <td>${displayDuration(
                      (block.end || this.endTime).diff(this.startTime),
                    )}</td>
                  </tr>
                </div>
              `,
          tooltip,
        );

        window.dispatchEvent(
          new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
            detail: {
              tooltip,
              targetRect: (e.target as HTMLElement).getBoundingClientRect(),
              gapSize: 5,
            },
          }),
        );
      })
      .on('mouseout', () => {
        window.dispatchEvent(
          new CustomEvent<HideTooltipEventDetail>('hide-tooltip', {
            detail: { delay: 0 },
          }),
        );
      });
  }
  static styles = [
    commonStyles,
    css`
      #timeline {
        display: grid;
        grid-template-rows: ${TOP_AXIS_HEIGHT}px 1fr ${BOTTOM_AXIS_HEIGHT}px;
        grid-template-columns: ${SIDE_PANEL_WIDTH}px 1fr;
        grid-template-areas:
          'header header'
          'side-panel body'
          'footer footer';
      }

      #header {
        grid-area: header;
        position: sticky;
        top: 0;
        background: white;
        z-index: 2;
      }

      #footer {
        grid-area: footer;
        position: sticky;
        bottom: 0;
        background: white;
        z-index: 2;
      }

      #side-panel {
        grid-area: side-panel;
        z-index: 1;
        font-weight: 500;
      }

      #body {
        grid-area: body;
      }

      #body path.domain {
        stroke: none;
      }

      svg {
        width: 100%;
        height: 100%;
      }

      text {
        fill: var(--default-text-color);
      }

      #relative-time {
        fill: red;
      }

      .grid line {
        stroke: var(--divider-color);
      }

      .clickable {
        cursor: pointer;
      }
      .not-intractable {
        pointer-events: none;
      }
      .hyperlink {
        text-decoration: underline;
      }
      .nowrap {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      .scheduled > rect {
        stroke: var(--scheduled-color);
        fill: var(--scheduled-bg-color);
      }
      .started > rect {
        stroke: var(--started-color);
        fill: var(--started-bg-color);
      }
      .success > rect {
        stroke: var(--success-color);
        fill: var(--success-bg-color);
      }
      .failure > rect {
        stroke: var(--failure-color);
        fill: var(--failure-bg-color);
      }
      .infra-failure > rect {
        stroke: var(--critical-failure-color);
        fill: var(--critical-failure-bg-color);
      }
      .canceled > rect {
        stroke: var(--canceled-color);
        fill: var(--canceled-bg-color);
      }

      .invisible {
        opacity: 0;
      }
    `,
  ];
}
