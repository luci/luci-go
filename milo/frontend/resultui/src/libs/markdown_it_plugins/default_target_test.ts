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

import { fixture } from '@open-wc/testing';
import { assert } from 'chai';
import MarkdownIt from 'markdown-it';

import { defaultTarget } from './default_target';

const links = `
www.a.com
[b](http://www.b.com)
<www.c.com>
<a href="http://www.d.com">www.d.com</a>
`;

const md = MarkdownIt('zero', {linkify: true, html: true})
  .enable(['linkify', 'autolink', 'link', 'html_inline'])
  .use(defaultTarget, '_blank');

describe('default_target', async () => {
  const ele = await fixture(md.render(links));
  const anchors = ele.querySelectorAll('a');

  it('can set default target', () => {
    const anchor1 =  anchors.item(0);
    assert.equal(anchor1.target, '_blank');
    assert.equal(anchor1.href, 'http://www.a.com/');

    const anchor2 =  anchors.item(1);
    assert.equal(anchor2.target, '_blank');
    assert.equal(anchor2.href, 'http://www.b.com/');

    const anchor3 =  anchors.item(2);
    assert.equal(anchor3.target, '_blank');
    assert.equal(anchor3.href, 'http://www.c.com/');
  });

  it('does not set target on HTML links', () => {
    const anchor4 =  anchors.item(3);
    assert.notEqual(anchor4.target, '_blank');
    assert.equal(anchor4.href, 'http://www.d.com/');
  });
});
