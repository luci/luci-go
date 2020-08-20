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
    function groupTemplater(group: { data: { label: string; duration: string; } }): string {
      return `
        <div>
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
      <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/vis/4.16.1/vis.css">
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
  `;
}

customElement('milo-timeline-tab')(
  consumeBuildState(
    consumeAppState(TimelineTabElement),
  ),
);
