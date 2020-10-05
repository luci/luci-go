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

import { fixture, fixtureCleanup } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';

import { safeHrefHtml } from './safe_href_html';

describe('safeHrefHtml', () => {
  it('should sanitise dynamically assigned href attribute', async () => {
    after(fixtureCleanup);
    const anchor = await fixture<HTMLAnchorElement>(safeHrefHtml`
      <a id="link" href=${'javascript:alert(document.domain)'}>js</a>
    `);
    assert.equal(anchor.getAttribute('href'), '');
  });

  it('should sanitise dynamically constructed href attribute', async () => {
    after(fixtureCleanup);
    const anchor = await fixture<HTMLAnchorElement>(safeHrefHtml`
      <a id="link" href="javascript:alert${'(document.domain)'}">js</a>
    `);
    assert.equal(anchor.getAttribute('href'), '');
  });

  it('should not sanitise static href attribute', async () => {
    after(fixtureCleanup);
    const anchor = await fixture<HTMLAnchorElement>(safeHrefHtml`
      <a id="link" href="javascript:alert(document.domain)">js</a>
    `);
    assert.equal(anchor.getAttribute('href'), 'javascript:alert(document.domain)');
  });

  it('should not modify normal href attribute', async () => {
    after(fixtureCleanup);
    const anchor = await fixture<HTMLAnchorElement>(safeHrefHtml`
      <a id="link" href="https://www.google.com">js</a>
    `);
    assert.equal(anchor.getAttribute('href'), 'https://www.google.com');
  });
});
