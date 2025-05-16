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

import createDomPurify from 'dompurify';
import { trustedTypes } from 'trusted-types';

const domPurify = createDomPurify(window);

// Mitigate target="_blank" vulnerability.
domPurify.addHook('afterSanitizeAttributes', (node) => {
  if (!['A', 'FORM', 'AREA'].includes(node.tagName)) {
    return;
  }

  // Note: rel="noopener" is added when the target is not set because <base> can
  // set the default target to _blank.
  if (
    ['_self', '_top', '_parent'].includes(node.getAttribute('target') || '')
  ) {
    return;
  }

  const existingRef = node.getAttribute('rel') || '';
  if (!/\bnoopener\b/i.test(existingRef)) {
    node.setAttribute('rel', (existingRef + ' noopener').trim());
  }
});

/**
 * Sanitizes the input HTML string.
 */
export function sanitizeHTML(html: string): string;
export function sanitizeHTML(
  html: string,
  opts: { RETURN_TRUSTED_TYPE: true },
): TrustedHTML;
export function sanitizeHTML(
  html: string,
  opts?: { RETURN_TRUSTED_TYPE: true },
): string | TrustedHTML {
  return domPurify.sanitize(html, {
    ADD_ATTR: [
      'target',
      'artifact-id',
      'inv-level',
      'experimental-ansi-support',
    ],
    ADD_TAGS: ['text-artifact'],
    RETURN_TRUSTED_TYPE: opts?.RETURN_TRUSTED_TYPE,
  });
}

/**
 * Initialize a default trusted types policy.
 *
 * IMPORTANT:
 * The default policy may fail to sanitize the HTML when the correct CSP (
 * content security policy) is not set via a HTTP header or HTML meta tag. Given
 * that the code for setting the CSP is often located far away from where the
 * unsanitized HTMLs are used, it's hard to notice when the default policy
 * failed to be applied automatically. To avoid this issue, HTML should still be
 * sanitized before being injected to DOM (e.g. via React's
 * `dangerouslySetInnerHTML`, or setting `ele.innerHTML`) even when the
 * default trusted types policy is used.
 */
export function initDefaultTrustedTypesPolicy() {
  if (!window.trustedTypes || !window.trustedTypes.createPolicy) {
    window.trustedTypes = trustedTypes;
  }

  window.trustedTypes!.createPolicy('default', {
    createHTML: sanitizeHTML,
  });
}
