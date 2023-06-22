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

import { expect } from '@jest/globals';
import { fixture } from '@open-wc/testing-helpers';
import markdownIt from 'markdown-it';

import { crbugLink } from './crbug_link';

const crbugLinks = `
crbug/123 crbug.com/234 not a link CRBUG.COM/345
crbug/proj/456 crbug/proj/657not_a_link
`;

const md = markdownIt('zero', { linkify: true })
  .enable('linkify')
  .use(crbugLink);

describe('crbug_link', () => {
  test('can render links correctly', async () => {
    const ele = await fixture(md.render(crbugLinks));

    const anchors = ele.querySelectorAll('a');
    expect(anchors.length).toStrictEqual(4);

    const anchor1 = anchors.item(0);
    expect(anchor1.href).toStrictEqual('https://crbug.com/123');
    expect(anchor1.text).toStrictEqual('crbug/123');

    const anchor2 = anchors.item(1);
    expect(anchor2.href).toStrictEqual('https://crbug.com/234');
    expect(anchor2.text).toStrictEqual('crbug.com/234');

    const anchor3 = anchors.item(2);
    expect(anchor3.href).toStrictEqual('https://crbug.com/345');
    expect(anchor3.text).toStrictEqual('CRBUG.COM/345');

    const anchor4 = anchors.item(3);
    expect(anchor4.href).toStrictEqual('https://crbug.com/proj/456');
    expect(anchor4.text).toStrictEqual('crbug/proj/456');
  });
});
