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
import markdownIt from 'markdown-it';

import { bugnizerLink } from './bugnizer_link';

const bugnizerLinks = `
b:123 b/234 ab/345 b:proj/456 not a link
crbug/567
`;

const md = markdownIt('zero', { linkify: true })
  .enable('linkify')
  .use(bugnizerLink);

describe('bugnizer_link', () => {
  test('can render links correctly', async () => {
    const ele = await fixture(md.render(bugnizerLinks));

    const anchors = ele.querySelectorAll('a');
    expect(anchors.length).toStrictEqual(2);

    const anchor1 = anchors.item(0);
    expect(anchor1.href).toStrictEqual('http://b/123');
    expect(anchor1.text).toStrictEqual('b:123');

    const anchor2 = anchors.item(1);
    expect(anchor2.href).toStrictEqual('http://b/234');
    expect(anchor2.text).toStrictEqual('b/234');
  });
});
