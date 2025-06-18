// Copyright 2023 The LUCI Authors.
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
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import '@/generic_libs/components/expandable_entry';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { Artifact } from '@/common/services/resultdb';
import { commonStyles } from '@/common/styles/stylesheets';
import { logging } from '@/common/tools/logging';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

// Allowlist of hosts, used to validate URLs specified in the contents of link
// artifacts. If the URL specified in a link artifact is not in this allowlist,
// the original fetch URL for the artifact will be returned instead.
const LINK_ARTIFACT_HOST_ALLOWLIST = [
  'cros-test-analytics.appspot.com', // Testhaus logs
  'stainless.corp.google.com', // Stainless logs
  'tests.chromeos.goog', // Preferred. A hostname alias for Testhaus logs.
];

/**
 * Renders a link artifact.
 */
@customElement('milo-link-artifact')
export class LinkArtifactElement extends MobxLitElement {
  @observable.ref artifact!: Artifact;
  @observable.ref label?: string;

  @observable.ref private loadError = false;

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
      ).then((res) => {
        if (!res.ok) {
          this.loadError = true;
          return '';
        }
        return res.text();
      }),
    );
  }

  @computed
  private get content() {
    const content = unwrapObservable(this.content$, null);
    if (content) {
      const url = new URL(content);
      const allowedProtocol = ['http:', 'https:'].includes(url.protocol);
      const allowedHost = LINK_ARTIFACT_HOST_ALLOWLIST.includes(url.host);
      if (!allowedProtocol || !allowedHost) {
        logging.warn(
          `Invalid target URL for link artifact ${this.artifact.name} - ` +
            'returning the original fetch URL for the artifact instead',
        );
        return this.artifact.fetchUrl;
      }
    }
    return content;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    if (this.loadError) {
      return html` <span class="load-error">
        Error loading ${this.artifact.artifactId} link
      </span>`;
    }

    if (this.content) {
      const linkText = this.label || this.artifact.artifactId;
      return html`<a href=${this.content} target="_blank">${linkText}</a>`;
    }

    return html`<span class="greyed-out">Loading...</span>`;
  });

  static styles = [
    commonStyles,
    css`
      .greyed-out {
        color: var(--greyed-out-text-color);
      }

      .load-error {
        color: var(--failure-color);
      }
    `,
  ];
}
