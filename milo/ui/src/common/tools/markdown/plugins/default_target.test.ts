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
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import markdownIt from 'markdown-it';

import { sanitizeHTML } from '../../sanitize_html';

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

const md = markdownIt('zero', { linkify: true, html: true })
  .enable(['linkify', 'autolink', 'link', 'html_inline', 'html_block'])
  .use(defaultTarget, '_blank');

describe('default_target', () => {
  let anchors: NodeListOf<HTMLAnchorElement>;
  beforeAll(async () => {
    const ele = await fixture(
      html`<div>${unsafeHTML(sanitizeHTML(md.render(links)))}</div>`,
    );
    anchors = ele.querySelectorAll('a');
  });

  it('can set default target', () => {
    const anchor1 = anchors.item(0);
    expect(anchor1.target).toStrictEqual('_blank');
    expect(anchor1.href).toStrictEqual('http://www.a.com/');

    const anchor2 = anchors.item(1);
    expect(anchor2.target).toStrictEqual('_blank');
    expect(anchor2.href).toStrictEqual('http://www.b.com/');

    const anchor3 = anchors.item(2);
    expect(anchor3.target).toStrictEqual('_blank');
    expect(anchor3.href).toStrictEqual('http://www.c.com/');

    const anchor4 = anchors.item(3);
    expect(anchor4.target).toStrictEqual('_blank');
    expect(anchor4.href).toStrictEqual('http://www.d.com/');

    const anchor5 = anchors.item(5);
    expect(anchor5.target).toStrictEqual('_blank');
    expect(anchor5.href).toStrictEqual('http://www.f.com/');
  });

  test('does not override target', () => {
    const anchor4 = anchors.item(4);
    expect(anchor4.target).toStrictEqual('_parent');
    expect(anchor4.href).toStrictEqual('http://www.e.com/');

    const anchor5 = anchors.item(6);
    expect(anchor5.target).toStrictEqual('_self');
    expect(anchor5.href).toStrictEqual('http://www.g.com/');
  });
});
