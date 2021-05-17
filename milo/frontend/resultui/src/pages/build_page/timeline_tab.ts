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

import * as d3 from 'd3';
import { css, customElement, html, property } from 'lit-element';
import { autorun, observable } from 'mobx';

import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportError } from '../../libs/error_handler';
import commonStyle from '../../styles/common_style.css';

const MARGIN = 10;
const AXIS_HEIGHT = 20;
const BORDER_SIZE = 1;

const ROW_HEIGHT = 30;

const SIDE_PANEL_WIDTH = 450;
const MIN_GRAPH_WIDTH = 500 + SIDE_PANEL_WIDTH;

@customElement('milo-timeline-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class TimelineTabElement extends MiloBaseElement {
  @observable.ref
  @consumeAppState
  appState!: AppState;

  @observable.ref
  @consumeBuildState
  buildState!: BuildState;

  @observable.ref private totalWidth!: number;
  @observable.ref private bodyWidth!: number;

  // Don't set them as observable. When render methods update them, we don't
  // want autorun to trigger this.renderTimeline() again.
  @property() private headerEle!: HTMLDivElement;
  @property() private footerEle!: HTMLDivElement;
  @property() private sidePanelEle!: HTMLDivElement;
  @property() private bodyEle!: HTMLDivElement;

  // Properties shared between render methods.
  private bodyHeight!: number;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'timeline';
    trackEvent(GA_CATEGORIES.TIMELINE_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);

    const syncWidth = () => {
      this.totalWidth = Math.max(window.innerWidth - 2 * MARGIN, MIN_GRAPH_WIDTH);
      this.bodyWidth = this.totalWidth - SIDE_PANEL_WIDTH;
    };
    window.addEventListener('resize', syncWidth);
    this.addDisposer(() => window.removeEventListener('resize', syncWidth));
    syncWidth();

    this.addDisposer(autorun(() => this.renderTimeline()));
  }

  protected render() {
    return html`${this.sidePanelEle}${this.headerEle}${this.bodyEle}${this.footerEle}`;
  }

  private renderTimeline = reportError.bind(this)(() => {
    const build = this.buildState.build;
    if (!build) {
      return;
    }

    this.bodyHeight = build.steps.length * ROW_HEIGHT - BORDER_SIZE;

    // Render each component.
    this.renderHeader();
    this.renderFooter();
    this.renderSidePanel();
    this.renderBody();
  });

  private renderHeader() {
    this.headerEle = document.createElement('div');
    d3.select(this.headerEle)
      .attr('id', 'header')
      .append('svg')
      .attr('viewport', `0 0 ${this.totalWidth} ${AXIS_HEIGHT}`);
  }

  private renderFooter() {
    this.footerEle = document.createElement('div');
    d3.select(this.footerEle)
      .attr('id', 'footer')
      .append('svg')
      .attr('viewport', `0 0 ${this.totalWidth} ${AXIS_HEIGHT}`);
  }

  private renderSidePanel() {
    this.sidePanelEle = document.createElement('div');
    d3.select(this.sidePanelEle)
      .style('width', SIDE_PANEL_WIDTH + 'px')
      .style('height', this.bodyHeight + 'px')
      .attr('id', 'side-panel')
      .append('svg')
      .attr('viewport', `0 0 ${SIDE_PANEL_WIDTH} ${this.bodyHeight}`);
  }

  private renderBody() {
    this.bodyEle = document.createElement('div');
    d3.select(this.bodyEle)
      .style('width', this.bodyWidth + 'px')
      .style('height', this.bodyHeight + 'px')
      .attr('id', 'body')
      .append('svg')
      .attr('viewport', `0 0 ${this.bodyWidth} ${this.bodyHeight}`);
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: grid;
        margin: ${MARGIN}px;
        grid-template-rows: ${AXIS_HEIGHT}px 1fr ${AXIS_HEIGHT}px;
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

      svg {
        width: 100%;
        height: 100%;
      }
    `,
  ];
}
