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

import { fixture } from '@open-wc/testing-helpers';
import { assert } from 'chai';
import MarkdownIt from 'markdown-it';

import { crbugLink } from './crbug_link';

const crbugLinks = `
crbug/123 crbug.com/234 not a link CRBUG.COM/345
crbug/proj/456 crbug/proj/657not_a_link
`;

const md = MarkdownIt('zero', { linkify: true }).enable('linkify').use(crbugLink);

describe('crbug_link', () => {
  it('can render links correctly', async () => {
    const ele = await fixture(md.render(crbugLinks));

    const anchors = ele.querySelectorAll('a');
    assert.equal(anchors.length, 4);

    const anchor1 = anchors.item(0);
    assert.equal(anchor1.href, 'https://crbug.com/123');
    assert.equal(anchor1.text, 'crbug/123');

    const anchor2 = anchors.item(1);
    assert.equal(anchor2.href, 'https://crbug.com/234');
    assert.equal(anchor2.text, 'crbug.com/234');

    const anchor3 = anchors.item(2);
    assert.equal(anchor3.href, 'https://crbug.com/345');
    assert.equal(anchor3.text, 'CRBUG.COM/345');

    const anchor4 = anchors.item(3);
    assert.equal(anchor4.href, 'https://crbug.com/proj/456');
    assert.equal(anchor4.text, 'crbug/proj/456');
  });
});
