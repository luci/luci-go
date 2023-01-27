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

import { bugnizerLink } from './bugnizer_link';

const bugnizerLinks = `
b:123 b/234 ab/345 b:proj/456 not a link
crbug/567
`;

const md = MarkdownIt('zero', { linkify: true }).enable('linkify').use(bugnizerLink);

describe('bugnizer_link', () => {
  it('can render links correctly', async () => {
    const ele = await fixture(md.render(bugnizerLinks));

    const anchors = ele.querySelectorAll('a');
    assert.equal(anchors.length, 2);

    const anchor1 = anchors.item(0);
    assert.equal(anchor1.href, 'http://b/123');
    assert.equal(anchor1.text, 'b:123');

    const anchor2 = anchors.item(1);
    assert.equal(anchor2.href, 'http://b/234');
    assert.equal(anchor2.text, 'b/234');
  });
});
