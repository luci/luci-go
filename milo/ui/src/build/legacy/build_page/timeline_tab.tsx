// Copyright 2020 The LUCI Authors.
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
import { autorun, makeObservable, observable } from 'mobx';

import '@/generic_libs/components/dot_spinner';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
} from '@/common/components/tooltip';
import { BUILD_STATUS_CLASS_MAP } from '@/common/constants/legacy';
import { PREDEFINED_TIME_INTERVALS } from '@/common/constants/time';
import { consumeStore, StoreInstance } from '@/common/store';
import { StepExt } from '@/common/store/build_state';
import { commonStyles } from '@/common/styles/stylesheets';
import {
  displayDuration,
  NUMERIC_TIME_FORMAT,
} from '@/common/tools/time_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import {
  errorHandler,
  forwardWithoutMsg,
  reportError,
  reportRenderError,
} from '@/generic_libs/tools/error_handler';
import { enumerate } from '@/generic_libs/tools/iter_utils';
import { consumer } from '@/generic_libs/tools/lit_context';
import { roundDown } from '@/generic_libs/tools/utils';

const MARGIN = 10;
const TOP_AXIS_HEIGHT = 35;
const BOTTOM_AXIS_HEIGHT = 25;
const BORDER_SIZE = 1;
const HALF_BORDER_SIZE = BORDER_SIZE / 2;

const ROW_HEIGHT = 30;
const STEP_HEIGHT = 24;
const STEP_MARGIN = (ROW_HEIGHT - STEP_HEIGHT) / 2 - HALF_BORDER_SIZE;
const STEP_EXTRA_WIDTH = 2;

const TEXT_HEIGHT = 10;
const STEP_TEXT_OFFSET = ROW_HEIGHT / 2 + TEXT_HEIGHT / 2;
const TEXT_MARGIN = 10;

const SIDE_PANEL_WIDTH = 400;
const MIN_GRAPH_WIDTH = 500 + SIDE_PANEL_WIDTH;
const SIDE_PANEL_RECT_WIDTH =
  SIDE_PANEL_WIDTH - STEP_MARGIN * 2 - BORDER_SIZE * 2;
const STEP_IDENT = 15;

const LIST_ITEM_WIDTH = SIDE_PANEL_RECT_WIDTH - TEXT_MARGIN * 2;
const LIST_ITEM_HEIGHT = 16;
const LIST_ITEM_X_OFFSET = STEP_MARGIN + TEXT_MARGIN + BORDER_SIZE;
const LIST_ITEM_Y_OFFSET = STEP_MARGIN + (STEP_HEIGHT - LIST_ITEM_HEIGHT) / 2;

const V_GRID_LINE_MAX_GAP = 80;

@customElement('milo-timeline-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class TimelineTabElement extends MobxExtLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref private totalWidth!: number;
  @observable.ref private bodyWidth!: number;

  @observable.ref private headerEle!: HTMLDivElement;
  @observable.ref private footerEle!: HTMLDivElement;
  @observable.ref private sidePanelEle!: HTMLDivElement;
  @observable.ref private bodyEle!: HTMLDivElement;

  // Properties shared between render methods.
  private bodyHeight!: number;
  private scaleTime!: d3.ScaleTime<number, number, never>;
  private scaleStep!: d3.ScaleLinear<number, number, never>;
  private timeInterval!: d3.TimeInterval;
  private readonly nowTimestamp = Date.now();
  private readonly now = DateTime.fromMillis(this.nowTimestamp);
  private relativeTimeText!: Selection<
    SVGTextElement,
    unknown,
    null,
    undefined
  >;

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    const syncWidth = () => {
      this.totalWidth = Math.max(
        window.innerWidth - 2 * MARGIN,
        MIN_GRAPH_WIDTH,
      );
      this.bodyWidth = this.totalWidth - SIDE_PANEL_WIDTH;
    };
    window.addEventListener('resize', syncWidth);
    this.addDisposer(() => window.removeEventListener('resize', syncWidth));
    syncWidth();

    this.addDisposer(autorun(() => this.renderTimeline()));
  }

  protected render = reportRenderError(this, () => {
    if (!this.store.buildPage.build) {
      return html`<div id="load">Loading <milo-dot-spinner></milo-load-spinner></div>`;
    }

    if (this.store.buildPage.build.steps.length === 0) {
      return html`<div id="no-steps">No steps were run.</div>`;
    }

    return html`<div id="timeline">
      ${this.sidePanelEle}${this.headerEle}${this.bodyEle}${this.footerEle}
    </div>`;
  });

  private renderTimeline = reportError(this, () => {
    const build = this.store.buildPage.build;
    if (!build || !build.startTime || build.steps.length === 0) {
      return;
    }

    const startTime = build.startTime.toMillis();
    const endTime = build.endTime?.toMillis() || this.nowTimestamp;

    this.bodyHeight = build.steps.length * ROW_HEIGHT - BORDER_SIZE;
    const padding =
      Math.ceil(((endTime - startTime) * STEP_EXTRA_WIDTH) / this.bodyWidth) /
      2;

    // Calc attributes shared among components.
    this.scaleTime = scaleTime()
      // Add a bit of padding to ensure everything renders in the viewport.
      .domain([startTime - padding, endTime + padding])
      // Ensure the right border is rendered within the viewport, while the left
      // border overlaps with the right border of the side-panel.
      .range([-HALF_BORDER_SIZE, this.bodyWidth - HALF_BORDER_SIZE]);
    this.scaleStep = scaleLinear()
      .domain([0, build.steps.length])
      // Ensure the top and bottom borders are not rendered.
      .range([-HALF_BORDER_SIZE, this.bodyHeight + HALF_BORDER_SIZE]);

    const maxInterval =
      (endTime - startTime + 2 * padding) /
      (this.bodyWidth / V_GRID_LINE_MAX_GAP);

    this.timeInterval = timeMillisecond.every(
      roundDown(maxInterval, PREDEFINED_TIME_INTERVALS),
    )!;

    // Render each component.
    this.renderHeader();
    this.renderFooter();
    this.renderSidePanel();
    this.renderBody();
  });

  private renderHeader() {
    const build = this.store.buildPage.build!;

    this.headerEle = document.createElement('div');
    const svg = d3Select(this.headerEle)
      .attr('id', 'header')
      .append('svg')
      .attr('viewport', `0 0 ${this.totalWidth} ${TOP_AXIS_HEIGHT}`);

    svg
      .append('text')
      .attr('x', TEXT_MARGIN)
      .attr('y', TOP_AXIS_HEIGHT - TEXT_MARGIN / 2)
      .attr('font-weight', '500')
      .text(
        'Build Start Time: ' + build.startTime!.toFormat(NUMERIC_TIME_FORMAT),
      );

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
    const build = this.store.buildPage.build!;

    this.footerEle = document.createElement('div');
    const svg = d3Select(this.footerEle)
      .attr('id', 'footer')
      .append('svg')
      .attr('viewport', `0 0 ${this.totalWidth} ${BOTTOM_AXIS_HEIGHT}`);

    if (build.endTime) {
      svg
        .append('text')
        .attr('x', TEXT_MARGIN)
        .attr('y', TEXT_HEIGHT + TEXT_MARGIN / 2)
        .attr('font-weight', '500')
        .text('Build End Time: ' + build.endTime.toFormat(NUMERIC_TIME_FORMAT));
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
    const build = this.store.buildPage.build!;

    this.sidePanelEle = document.createElement('div');
    const svg = d3Select(this.sidePanelEle)
      .style('width', SIDE_PANEL_WIDTH + 'px')
      .style('height', this.bodyHeight + 'px')
      .attr('id', 'side-panel')
      .append('svg')
      .attr('viewport', `0 0 ${SIDE_PANEL_WIDTH} ${this.bodyHeight}`);

    // Grid lines
    const horizontalGridLines = axisLeft(this.scaleStep)
      .ticks(build.steps.length)
      .tickFormat(() => '')
      .tickSize(-SIDE_PANEL_WIDTH)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(horizontalGridLines);

    for (const [i, step] of enumerate(build.steps)) {
      const stepGroup = svg
        .append('g')
        .attr('class', BUILD_STATUS_CLASS_MAP[step.status])
        .attr('transform', `translate(0, ${i * ROW_HEIGHT})`);

      const rect = stepGroup
        .append('rect')
        .attr('x', STEP_MARGIN + BORDER_SIZE)
        .attr('y', STEP_MARGIN)
        .attr('width', SIDE_PANEL_RECT_WIDTH)
        .attr('height', STEP_HEIGHT);
      this.installStepInteractionHandlers(rect, step);

      const listItem = stepGroup
        .append('foreignObject')
        .attr('class', 'not-intractable')
        .attr('x', LIST_ITEM_X_OFFSET + step.depth * STEP_IDENT)
        .attr('y', LIST_ITEM_Y_OFFSET)
        .attr('height', STEP_HEIGHT - LIST_ITEM_Y_OFFSET)
        .attr('width', LIST_ITEM_WIDTH);
      listItem.append('xhtml:span').text(step.listNumber + ' ');
      const stepText = listItem.append('xhtml:span').text(step.selfName);

      if (step.logs[0]?.viewUrl) {
        stepText.attr('class', 'hyperlink');
      }
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
    const build = this.store.buildPage.build!;

    this.bodyEle = document.createElement('div');
    const svg = d3Select(this.bodyEle)
      .attr('id', 'body')
      .style('width', this.bodyWidth + 'px')
      .style('height', this.bodyHeight + 'px')
      .append('svg')
      .attr('viewport', `0 0 ${this.bodyWidth} ${this.bodyHeight}`);

    // Grid lines
    const verticalGridLines = axisTop(this.scaleTime)
      .ticks(this.timeInterval)
      .tickSize(-this.bodyHeight)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(verticalGridLines);
    const horizontalGridLines = axisLeft(this.scaleStep)
      .ticks(build.steps.length)
      .tickFormat(() => '')
      .tickSize(-this.bodyWidth)
      .tickFormat(() => '');
    svg.append('g').attr('class', 'grid').call(horizontalGridLines);

    for (const [i, step] of enumerate(build.steps)) {
      const start = this.scaleTime(
        step.startTime?.toMillis() || this.nowTimestamp,
      );
      const end = this.scaleTime(step.endTime?.toMillis() || this.nowTimestamp);

      const stepGroup = svg
        .append('g')
        .attr('class', BUILD_STATUS_CLASS_MAP[step.status])
        .attr('transform', `translate(${start}, ${i * ROW_HEIGHT})`);

      // Add extra width so tiny steps are visible.
      const width = end - start + STEP_EXTRA_WIDTH;

      stepGroup
        .append('rect')
        .attr('x', -STEP_EXTRA_WIDTH / 2)
        .attr('y', STEP_MARGIN)
        .attr('width', width)
        .attr('height', STEP_HEIGHT);

      const isWide = width > this.bodyWidth * 0.33;
      const nearEnd = end > this.bodyWidth * 0.66;

      const stepText = stepGroup
        .append('text')
        .attr('text-anchor', isWide || !nearEnd ? 'start' : 'end')
        .attr(
          'x',
          isWide ? TEXT_MARGIN : nearEnd ? -TEXT_MARGIN : width + TEXT_MARGIN,
        )
        .attr('y', STEP_TEXT_OFFSET)
        .text(step.listNumber + ' ' + step.selfName);

      // Wail until the next event cycle so stepText is rendered when we call
      // this.getBBox();
      window.setTimeout(() => {
        // Rebind this so we can access it in the function below.
        const timelineTab = this; // eslint-disable-line @typescript-eslint/no-this-alias

        stepText.each(function () {
          // This is the standard d3 API.
          // eslint-disable-next-line no-invalid-this
          const textBBox = this.getBBox();
          const x1 = Math.min(textBBox.x, -STEP_EXTRA_WIDTH / 2);
          const x2 = Math.max(textBBox.x + textBBox.width, STEP_MARGIN + width);

          // This makes the step text easier to interact with.
          const eventTargetRect = stepGroup
            .append('rect')
            .attr('x', x1)
            .attr('y', STEP_MARGIN)
            .attr('width', x2 - x1)
            .attr('height', STEP_HEIGHT)
            .attr('class', 'invisible');

          timelineTab.installStepInteractionHandlers(eventTargetRect, step);
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
      const duration = time.diff(build.startTime!);
      this.relativeTimeText.attr('x', x);
      this.relativeTimeText.text(
        displayDuration(duration) + ' since build start',
      );
    });

    // Right border.
    svg
      .append('line')
      .attr('x1', this.bodyWidth - HALF_BORDER_SIZE)
      .attr('x2', this.bodyWidth - HALF_BORDER_SIZE)
      .attr('y2', this.bodyHeight)
      .attr('stroke', 'var(--default-text-color)');
  }

  /**
   * Installs handlers for interacting with a step object.
   */
  private installStepInteractionHandlers<T extends BaseType>(
    ele: Selection<T, unknown, null, undefined>,
    step: StepExt,
  ) {
    const logUrl = step.logs[0]?.viewUrl;
    if (logUrl) {
      ele
        .attr('class', ele.attr('class') + ' clickable')
        .on('click', (e: MouseEvent) => {
          e.stopPropagation();
          window.open(logUrl, '_blank');
        });
    }

    ele
      .on('mouseover', (e: MouseEvent) => {
        const tooltip = document.createElement('div');
        render(
          html`
            <table>
              <tr>
                <td colspan="2">${
                  logUrl
                    ? 'Click to open associated log.'
                    : html`<b>No associated log.</b>`
                }</td>
              </tr>
              <tr>
                <td>Started:</td>
                <td>
                  ${(step.startTime || this.now).toFormat(NUMERIC_TIME_FORMAT)}
                  (after ${displayDuration(
                    (step.startTime || this.now).diff(
                      this.store.buildPage.build!.startTime!,
                    ),
                  )})
                </td>
              </tr>
              <tr>
                <td>Ended:</td>
                <td>${
                  step.endTime
                    ? step.endTime.toFormat(NUMERIC_TIME_FORMAT) +
                      ` (after ${displayDuration(
                        step.endTime.diff(
                          this.store.buildPage.build!.startTime!,
                        ),
                      )})`
                    : 'N/A'
                }</td>
              </tr>
              <tr>
                <td>Duration:</td>
                <td>${displayDuration(step.duration)}</td>
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
      :host {
        display: block;
        margin: ${MARGIN}px;
      }

      #load {
        color: var(--active-text-color);
      }

      #timeline {
        display: grid;
        grid-template-rows: ${TOP_AXIS_HEIGHT}px 1fr ${BOTTOM_AXIS_HEIGHT}px;
        grid-template-columns: ${SIDE_PANEL_WIDTH}px 1fr;
        grid-template-areas:
          'header header'
          'side-panel body'
          'footer footer';
        margin-top: ${-MARGIN}px;
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-timeline-tab': Record<string, never>;
    }
  }
}

export function TimelineTab() {
  return <milo-timeline-tab />;
}

export function Component() {
  useTabId('timeline');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="timeline">
      <TimelineTab />
    </RecoverableErrorBoundary>
  );
}
