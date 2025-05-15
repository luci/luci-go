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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { reaction } from 'mobx';

import '@/generic_libs/components/dot_spinner';
import { LoadingStage } from '@/common/models/test_loader';
import {
  consumeInvocationState,
  InvocationStateInstance,
} from '@/common/store/invocation_state';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import {
  errorHandler,
  forwardWithoutMsg,
  reportErrorAsync,
} from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';

/**
 * Format number with a cap.
 */
function formatNum(num: number, hasMore: boolean, cap?: number) {
  if (cap && num > cap) {
    return `${cap}+`;
  } else if (hasMore) {
    return `${num}+`;
  }
  return `${num}`;
}

/**
 * Renders the number of most severe test failures in a badge.
 */
@customElement('milo-trt-count-indicator')
@errorHandler(forwardWithoutMsg, () => html``)
@consumer
export class TestResultsTabCountIndicatorElement extends MobxExtLitElement {
  @consumeInvocationState() invState!: InvocationStateInstance;

  connectedCallback() {
    super.connectedCallback();

    // When a new test loader is received, load the first page.
    this.addDisposer(
      reaction(
        () => this.invState.testLoader,
        (testLoader) =>
          reportErrorAsync(this, async () =>
            testLoader?.loadFirstPageOfTestVariants(),
          )(),
        { fireImmediately: true },
      ),
    );
  }

  protected render() {
    const testLoader = this.invState.testLoader;
    if (!testLoader?.firstPageLoaded) {
      return html`<milo-dot-spinner></milo-dot-spinner>`;
    }

    if (testLoader.unfilteredUnexpectedVariantsCount > 0) {
      return html`<div
        id="unexpected"
        title=${formatNum(
          testLoader.unfilteredUnexpectedVariantsCount,
          !testLoader.loadedAllUnexpectedVariants,
        ) + ' unexpected tests'}
      >
        ${formatNum(
          testLoader.unfilteredUnexpectedVariantsCount,
          !testLoader.loadedAllUnexpectedVariants,
          99,
        )}
      </div>`;
    }

    if (testLoader.unfilteredUnexpectedlySkippedVariantsCount > 0) {
      return html`<div
        id="unexpectedly-skipped"
        title=${formatNum(
          testLoader.unfilteredUnexpectedlySkippedVariantsCount,
          !testLoader.loadedAllUnexpectedVariants,
        ) + ' unexpectedly skipped tests'}
      >
        ${formatNum(
          testLoader.unfilteredUnexpectedlySkippedVariantsCount,
          testLoader.stage < LoadingStage.LoadingUnexpectedlySkipped,
        )}
      </div>`;
    }

    if (testLoader.unfilteredFlakyVariantsCount > 0) {
      return html`<div
        id="flaky"
        title=${formatNum(
          testLoader.unfilteredFlakyVariantsCount,
          !testLoader.loadedAllUnexpectedVariants,
        ) + ' flaky tests'}
      >
        ${formatNum(
          testLoader.unfilteredFlakyVariantsCount,
          testLoader.stage < LoadingStage.LoadingFlaky,
        )}
      </div>`;
    }

    if (testLoader.unfilteredTestVariantCount > 0) {
      return html`<mwc-icon id="expected" title="all tests passed"
        >check_circle</mwc-icon
      >`;
    }

    return;
  }

  static styles = css`
    :host {
      display: inline-block;
      width: 30px;
    }

    milo-dot-spinner {
      color: var(--active-color);
      width: 30px;
      font-size: 13px;
    }

    div {
      color: white;
      display: inline-block;
      padding: 0.1em;
      padding-top: 0.2em;
      font-size: 75%;
      font-weight: 700;
      line-height: 13px;
      text-align: center;
      white-space: nowrap;
      vertical-align: bottom;
      border-radius: 0.25rem;
      margin-bottom: 2px;
      box-sizing: border-box;
      width: 30px;
    }
    #unexpected {
      background-color: var(--failure-color);
    }
    #expectedly-skipped {
      background-color: var(--critical-failure-color);
    }
    #flaky {
      background-color: var(--warning-color);
    }

    mwc-icon {
      --mdc-icon-size: 18px;
      vertical-align: bottom;
      width: 20px;
      margin-right: 10px;
    }
    #expected {
      color: var(--success-color);
    }
  `;
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      ['milo-trt-count-indicator']: {
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface CountIndicatorProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function CountIndicator(props: CountIndicatorProps) {
  return <milo-trt-count-indicator {...props} class={props.className} />;
}
