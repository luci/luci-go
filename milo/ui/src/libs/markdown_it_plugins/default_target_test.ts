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

import { fixture, html } from '@open-wc/testing-helpers';
import { assert } from 'chai';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import MarkdownIt from 'markdown-it';

import { defaultTarget } from './default_target';

const links = `
www.a.com
[b](http://www.b.com)
<www.c.com>
<a href="http://www.d.com">www.d.com</a>
<a href="http://www.e.com" target="_parent">www.e.com</a>

<div>
  <a href="http://www.f.com">
    www.f.com
  </a>
  <a href="http://www.g.com" target="_self">
    www.g.com
  </a>
</div>
`;

const md = MarkdownIt('zero', { linkify: true, html: true })
  .enable(['linkify', 'autolink', 'link', 'html_inline', 'html_block'])
  .use(defaultTarget, '_blank');

describe('default_target', () => {
  let anchors: NodeListOf<HTMLAnchorElement>;
  before(async () => {
    const ele = await fixture(html`<div>${unsafeHTML(md.render(links))}</div>`);
    anchors = ele.querySelectorAll('a');
  });

  it('can set default target', () => {
    const anchor1 = anchors.item(0);
    assert.equal(anchor1.target, '_blank');
    assert.equal(anchor1.href, 'http://www.a.com/');

    const anchor2 = anchors.item(1);
    assert.equal(anchor2.target, '_blank');
    assert.equal(anchor2.href, 'http://www.b.com/');

    const anchor3 = anchors.item(2);
    assert.equal(anchor3.target, '_blank');
    assert.equal(anchor3.href, 'http://www.c.com/');

    const anchor4 = anchors.item(3);
    assert.equal(anchor4.target, '_blank');
    assert.equal(anchor4.href, 'http://www.d.com/');

    const anchor5 = anchors.item(5);
    assert.equal(anchor5.target, '_blank');
    assert.equal(anchor5.href, 'http://www.f.com/');
  });

  it('does not override target', () => {
    const anchor4 = anchors.item(4);
    assert.equal(anchor4.target, '_parent');
    assert.equal(anchor4.href, 'http://www.e.com/');

    const anchor5 = anchors.item(6);
    assert.equal(anchor5.target, '_self');
    assert.equal(anchor5.href, 'http://www.g.com/');
  });
});
