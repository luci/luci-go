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

import '@material/mwc-icon';

import '@/common/components/sanitized_html';
import '@/generic_libs/components/expandable_entry';

import { MobxLitElement } from '@adobe/lit-mobx';
import * as Diff2Html from 'diff2html';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { Artifact } from '@/common/services/resultdb';
import { commonStyles } from '@/common/styles/stylesheets';
import {
  getRawArtifactURLPath,
  getTextDiffArtifactURLPath,
} from '@/common/tools/url_utils';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

/**
 * Renders a text diff artifact.
 */
@customElement('milo-text-diff-artifact')
export class TextDiffArtifactElement extends MobxLitElement {
  @observable.ref artifact!: Artifact;

  @computed
  private get content$(): IPromiseBasedObservable<string> {
    return fromPromise(
      // TODO(crbug/1206109): use permanent raw artifact URL.
      fetch(
        urlSetSearchQueryParam(
          this.artifact.fetchUrl,
          'n',
          ARTIFACT_LENGTH_LIMIT,
        ),
      ).then((res) => res.text()),
    );
  }

  @computed
  private get content() {
    return unwrapObservable(this.content$, null);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    return html`
      <link
        rel="stylesheet"
        type="text/css"
        href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css"
      />
      <milo-expandable-entry .expanded=${true} .contentRuler="invisible">
        <span id="header" slot="header">
          Unexpected text output from
          <a
            href=${getTextDiffArtifactURLPath(this.artifact.name)}
            target="_blank"
          >
            ${this.artifact.artifactId}
          </a>
          (<a href=${getRawArtifactURLPath(this.artifact.name)} target="_blank"
            >view raw</a
          >)
        </span>
        <div id="content" slot="content">
          <milo-sanitized-html
            html=${Diff2Html.html(this.content || '', { drawFileList: false })}
          >
          </milo-sanitized-html>
        </div>
      </milo-expandable-entry>
    `;
  });

  static styles = [
    commonStyles,
    css`
      #content {
        padding-top: 5px;
        position: relative;
      }
      .d2h-code-linenumber {
        cursor: default;
      }
      .d2h-moved-tag {
        display: none;
      }
    `,
  ];
}
