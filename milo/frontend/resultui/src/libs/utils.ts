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

import { IPromiseBasedObservable, PENDING, REJECTED } from 'mobx-utils';

/**
 * Extend URL with methods that can be chained.
 */
export class ChainableURL extends URL {
  withSearchParam(key: string, value: string, override = false) {
    if (override) {
      this.searchParams.set(key, value);
    } else {
      this.searchParams.append(key, value);
    }
    return this;
  }
}

// Generates URL for collecting feedback.
export function genFeedbackUrl() {
  const feedbackComment =
    `From Link: ${document.location.href}\n` +
    'Please enter a description of the problem, with repro steps if applicable.';

  const searchParams = new URLSearchParams({
    template: 'Build Infrastructure',
    components: 'Infra>Platform>Milo>ResultUI',
    labels: 'Pri-2,Type-Bug',
    comment: feedbackComment,
  });
  return `https://bugs.chromium.org/p/chromium/issues/entry?${searchParams}`;
}

/**
 * Hash the message with SHA-256 and then return the outcome in hex encoded
 * string.
 */
export async function sha256(message: string) {
  const msgBuffer = new TextEncoder().encode(message);
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
  return hashHex;
}

/**
 * Returns a promise that resolves after the specified duration.
 */
export function timeout(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

/**
 * Unwraps the value in a promise based observable.
 *
 * If the observable is pending, return the defaultValue.
 * If the observable is rejected, throw the error.
 */
export function unwrapObservable<T>(observable: IPromiseBasedObservable<T>, defaultValue: T) {
  switch (observable.state) {
    case PENDING:
      return defaultValue;
    case REJECTED:
      throw observable.value;
    default:
      return observable.value;
  }
}
