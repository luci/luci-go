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

/**
 * @fileoverview
 * This is a replacement for the 'assertion-error' package.
 * Comparing to the 'assertion-error' package, this package supports native
 * rendering of the stack trace when debugging in a browser.
 */

// tslint:disable-next-line: no-default-export
module.exports = class AssertionError extends Error {
  actual: string;
  expected: string;
  showDiff: boolean;

  constructor(message: string, props: {actual: string, expected: string, showDiff: boolean}) {
    super(message);
    this.actual = props.actual;
    this.expected = props.expected;
    this.showDiff = props.showDiff;

    // Do not set name to 'AssertionError', otherwise the browser will not
    // render the stack trace.
    // this.name = 'AssertionError';
  }

  toJSON(stack: boolean) {
    return {
      name: this.name,
      ...stack ? {stack: this.stack} : {},
    };
  }
};
