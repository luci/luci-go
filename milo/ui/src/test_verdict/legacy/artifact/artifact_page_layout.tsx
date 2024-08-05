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

import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { useEffect, useRef } from 'react';
import { Outlet, useParams } from 'react-router-dom';

import '@/common/components/image_diff_viewer';
import '@/common/components/status_bar';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { ArtifactIdentifier } from '@/common/services/resultdb';
import { commonStyles } from '@/common/styles/stylesheets';
import { getInvURLPath } from '@/common/tools/url_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { createContextLink, provider } from '@/generic_libs/tools/lit_context';

export const [provideArtifactIdent, consumeArtifactIdent] =
  createContextLink<ArtifactIdentifier>();

/**
 * Renders the header of an artifact page.
 */
@customElement('milo-artifact-page-layout')
@provider
export class ArtifactPageLayoutElement extends MobxExtLitElement {
  @observable.ref private invId!: string;
  @observable.ref private testId?: string;
  @observable.ref private resultId?: string;
  @observable.ref private artifactId!: string;

  @computed
  @provideArtifactIdent()
  get artifactIdent() {
    return {
      invocationId: this.invId,
      testId: this.testId,
      resultId: this.resultId,
      artifactId: this.artifactId,
    };
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.artifactIdent,
        (artifactIdent) => {
          // Emulate @property() update.
          this.updated(new Map([['artifactIdent', artifactIdent]]));
        },
        { fireImmediately: true },
      ),
    );
  }

  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td>
              <a href=${getInvURLPath(this.invId)}> ${this.invId} </a>
            </td>
          </tr>
          ${this.testId &&
          html`
            <!-- TODO(weiweilin): add view test link -->
            <tr>
              <td class="id-component-label">Test</td>
              <td>${this.testId}</td>
            </tr>
          `}
          ${this.resultId &&
          html`
            <!-- TODO(weiweilin): add view result link -->
            <tr>
              <td class="id-component-label">Result</td>
              <td>${this.resultId}</td>
            </tr>
          `}
          <tr>
            <td class="id-component-label">Artifact</td>
            <td>${this.artifactId}</td>
          </tr>
        </table>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
      ></milo-status-bar>
      <slot></slot>
    `;
  }

  static styles = [
    commonStyles,
    css`
      :host {
        display: block;
      }

      #artifact-header {
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
      }
      .id-component-label {
        color: var(--light-text-color);
      }
    `,
  ];
}

export function ArtifactPageLayout() {
  const { invId, testId, resultId, artifactId } = useParams();

  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // This never happens, but useful for type narrowing.
    if (!container.current) {
      throw new Error('unreachable');
    }

    render(
      html` <milo-artifact-page-layout
        .invId=${invId}
        .testId=${testId}
        .resultId=${resultId}
        .artifactId=${artifactId}
      >
        ${container.current.children}
      </milo-artifact-page-layout>`,
      container.current,
    );
  }, [invId, testId, resultId, artifactId]);

  return (
    <div ref={container}>
      <div>
        <Outlet />
      </div>
    </div>
  );
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    // See the documentation for `<LoginPage />` for why we handle error this way.
    <RecoverableErrorBoundary key="artifact">
      <ArtifactPageLayout />
    </RecoverableErrorBoundary>
  );
}
