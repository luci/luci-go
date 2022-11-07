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

import { fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { unsafeHTML } from 'lit-html/directives/unsafe-html';

import { initDefaultTrustedTypesPolicy, sanitizeHTML } from './sanitize_html';

initDefaultTrustedTypesPolicy();

const DIRTY_HTML = `
<div>
  <a href="https://www.google.com" target="_blank"></a>
  <a href="https://www.google.com"></a>
  <a href="https://www.google.com" target="_self"></a>
  <a href="https://www.google.com" target="_blank" rel="nofollow"></a>
  <a href="https://www.google.com" target="_blank" rel="noopener nofollow"></a>
  <form target="_blank"></form>
  <area target="_blank"></area>
</div>
`;

describe('sanitize_html', () => {
  let root: Element;
  let anchors: NodeListOf<HTMLAnchorElement>;
  before(async () => {
    root = await fixture(html`${unsafeHTML(sanitizeHTML(DIRTY_HTML))}`);
    anchors = root.querySelectorAll('a');
  });
  after(fixtureCleanup);

  it('should set rel="noopener" when target attribute is set to _blank', () => {
    const anchor = anchors.item(0);
    assert.equal(anchor.getAttribute('rel'), 'noopener');
  });

  it('should set rel="noopener" when target attribute is not set', () => {
    const anchor = anchors.item(1);
    assert.equal(anchor.getAttribute('rel'), 'noopener');
  });

  it('should not set rel="noopener" when target attribute is set but not _blank', () => {
    const anchor = anchors.item(2);
    assert.equal(anchor.getAttribute('rel'), null);
  });

  it('should append to the existing rel attribute', () => {
    const anchor = anchors.item(3);
    assert.equal(anchor.getAttribute('rel'), 'nofollow noopener');
  });

  it('should not set rel="noopener" when it is already present', () => {
    const anchor = anchors.item(4);
    assert.equal(anchor.getAttribute('rel'), 'noopener nofollow');
  });

  it('should set rel="noopener" on <form> and <area> as well', () => {
    const form = root.querySelector('form')!;
    assert.equal(form.getAttribute('rel'), 'noopener');

    const area = root.querySelector('area')!;
    assert.equal(area.getAttribute('rel'), 'noopener');
  });
});
