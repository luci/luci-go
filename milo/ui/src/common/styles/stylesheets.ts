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

/**
 * @fileoverview
 *
 * This file copies CSS rules from other stylesheets and convert them into
 * Constructable Stylesheets so they can be used in Lit components.
 *
 * This helps us remove lit-css-loader from the dependency, which is not
 * available in Vite.
 *
 * TODO: remove this file once all elements have been converted to React.
 */

export const colorClasses = new CSSStyleSheet();

colorClasses.replace(`
/* v2 verdicts */
.failed-verdict {
  color: var(--failure-color);
}
.execution-errored-verdict {
  color: var(--critical-failure-color);
}
.precluded-verdict {
  color: var(--precluded-color);
}
.passed-verdict {
  color: var(--success-color);
}
.skipped-verdict {
  color: var(--skipped-color);
}
.flaky-verdict {
  color: var(--warning-color);
}
.exonerated-verdict {
  color: var(--exonerated-color);
}

/* v1 verdicts */
.unexpected {
  color: var(--failure-color);
}
.unexpectedly-skipped {
  color: var(--critical-failure-color);
}
.flaky {
  color: var(--warning-color);
}
span.flaky {
  color: var(--warning-text-color);
}
.exonerated {
  color: var(--exonerated-color);
}
.expected {
  color: var(--success-color);
}

.scheduled {
  color: var(--scheduled-color);
}
.started {
  color: var(--started-color);
}
.success {
  color: var(--success-color);
}
.failure {
  color: var(--failure-color);
}
.infra-failure {
  color: var(--critical-failure-color);
}
.canceled {
  color: var(--canceled-color);
}

.scheduled-bg {
  border: 1px solid var(--scheduled-color);
  background-color: var(--scheduled-bg-color);
}
.started-bg {
  border: 1px solid var(--started-color);
  background-color: var(--started-bg-color);
}
.success-bg {
  border: 1px solid var(--success-color);
  background-color: var(--success-bg-color);
}
.failure-bg {
  border: 1px solid var(--failure-color);
  background-color: var(--failure-bg-color);
}
.infra-failure-bg {
  border: 1px solid var(--critical-failure-color);
  background-color: var(--critical-failure-bg-color);
}
.canceled-bg {
  border: 1px solid var(--canceled-color);
  background-color: var(--canceled-bg-color);
}
`);

export const commonStyles = new CSSStyleSheet();

commonStyles.replace(`
a {
  color: var(--active-text-color);
}

.active-text {
  color: var(--active-text-color);
  cursor: pointer;
  font-size: 14px;
  font-weight: normal;
}

.duration {
  color: var(--light-text-color);
  background-color: var(--light-active-color);
  display: inline-block;
  padding: 0.25em 0.4em;
  font-size: 75%;
  font-weight: 500;
  line-height: 13px;
  text-align: center;
  white-space: nowrap;
  vertical-align: bottom;
  border-radius: 0.25rem;
  margin-bottom: 3px;
  width: 35px;
}

.duration.ms {
  background-color: var(--light-background-color-1);
}

.duration.s {
  background-color: var(--light-background-color-2);
}

.duration.m {
  background-color: var(--light-background-color-3);
}

.duration.h {
  background-color: var(--light-background-color-4);
}

.duration.d {
  color: white;
  background-color: var(--active-color);
}

input[type='checkbox'] {
  transform: translateY(1px);
}

mwc-icon {
  /* The icon font can take a while to load.
   * Set a default size to reduce page shifting.
   */
  width: var(--mdc-icon-size, 24px);
  height: var(--mdc-icon-size, 24px);
}

hr {
  background-color: var(--divider-color);
  border: none;
  height: 1px;
}
`);
