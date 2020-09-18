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


import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { autorun, observable } from 'mobx';
import { DataSet } from 'vis-data/peer';
import { Timeline } from 'vis-timeline/peer';

import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';

export class TimelineTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;
  @observable.ref rendered  = false;
  private disposers: Array<() => void> = [];

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'timeline';
    this.rendered = false;
    this.disposers.push(
      autorun(
        () => this.renderTimeline(),
      ),
    );
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  private renderTimeline() {
    if (this.buildState.buildPageData === null || !this.rendered) {
      return;
    }

    const container = this.shadowRoot!.getElementById('timeline')!;
    function groupTemplater(group: { data: { label: string; duration: string; statusClassName: string} }): string {
      return `
        <div class="group-title ${group.data.statusClassName}">
          <span class="title">${group.data.label}</span>
          <span class="duration">( ${group.data.duration} )</span>
        </div>
      `;
    }

    const options = {
      clickToUse: false,
      groupTemplate: groupTemplater,
      multiselect: false,
      orientation: {
        axis: 'both',
        item: 'top',
      },
      zoomable: false,
    };

    // Create a Timeline
    const timelineData = JSON.parse(this.buildState.buildPageData!.timeline);
    const timeline = new Timeline(
      container,
      new DataSet(timelineData.items),
      new DataSet(timelineData.groups),
      options,
    );

    timeline.on('select', (props) => {
      const item = timelineData.items[props.items[0]];
      if (!item) {
        return;
      }
      if (item.data && item.data.logUrl) {
        window.open(item.data.logUrl, '_blank');
      }
    });
  }

  protected render() {
    return html`
      <link rel="stylesheet" type="text/css" href="/static/styles/thirdparty/vis-timeline-graph2d.min.css">
      <div id="timeline"></div>
    `;
  }

  protected firstUpdated() {
    this.rendered = true;
  }

  static styles = css`
    .vis-range {
      cursor: pointer;
    }

    .status-INFRA_FAILURE {
      color: #FFFFFF;
      background-color: #c6c;
      border-color: #ACA0B3;
    }

    .status-RUNNING {
      color: #000;
      background-color: #fd3;
      border-color: #C5C56D;
    }

    .status-FAILURE {
      color: #000;
      background-color: #e88;
      border-color: #A77272;
      border-style: solid;
    }

    .status-CANCELED {
      color: #000;
      background-color: #8ef;
      border-color: #00d8fc;
      border-style: solid;
    }

    .status-SUCCESS {
      color: #000;
      background-color: #8d4;
      border-color: #4F8530;
    }

    .group-title > .title{
      display: inline-block;
      white-space: nowrap;
      max-width: 50em;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .group-title > .duration{
      display: inline-block;
      text-align: right;
      float: right;
    }

    .group-title {
      font-weight: bold;
      padding: 5px;
    }

    .vis-labelset .vis-label .vis-inner {
      display: block;
    }
  `;
}

customElement('milo-timeline-tab')(
  consumeBuildState(
    consumeAppState(TimelineTabElement),
  ),
);
