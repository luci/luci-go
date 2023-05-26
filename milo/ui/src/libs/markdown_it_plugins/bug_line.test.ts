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
import MarkdownIt from 'markdown-it';

import { bugLine } from './bug_line';

const singleBugLine = 'Bug: 123, 234, not a project, proj-1:345';
const multipleBugLinesWithSoftBreak = 'Bug: 123\nBUG:234';
const multipleBugLinesWithHardBreak = 'Bug: 123  \nBUG:234';

describe('bug_line', () => {
  it('can render single bug line correctly', async () => {
    const md = MarkdownIt('zero', { breaks: true }).enable('newline').use(bugLine);

    const ele = await fixture(md.render(singleBugLine));

    const anchors = ele.querySelectorAll('a');
    expect(anchors.length).toStrictEqual(3);

    const anchor1 = anchors.item(0);
    expect(anchor1.href).toStrictEqual('https://crbug.com/123');
    expect(anchor1.text).toStrictEqual('123');

    const anchor2 = anchors.item(1);
    expect(anchor2.href).toStrictEqual('https://crbug.com/234');
    expect(anchor2.text).toStrictEqual('234');

    const anchor3 = anchors.item(2);
    expect(anchor3.href).toStrictEqual('https://crbug.com/proj-1/345');
    expect(anchor3.text).toStrictEqual('proj-1:345');

    expect(ele.childNodes[4].textContent).toStrictEqual(', not a project, ');
  });

  describe('When breaks is set to true', () => {
    const md = MarkdownIt('zero', { breaks: true }).enable('newline').use(bugLine);

    it('can renders multiple bug lines with soft break correctly', async () => {
      const ele = await fixture(md.render(multipleBugLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      expect(anchors.length).toStrictEqual(2);

      const anchor1 = anchors.item(0);
      expect(anchor1.href).toStrictEqual('https://crbug.com/123');
      expect(anchor1.text).toStrictEqual('123');

      const anchor2 = anchors.item(1);
      expect(anchor2.href).toStrictEqual('https://crbug.com/234');
      expect(anchor2.text).toStrictEqual('234');
    });

    it('can renders multiple bug lines with hard break correctly', async () => {
      const ele = await fixture(md.render(multipleBugLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      expect(anchors.length).toStrictEqual(2);

      const anchor1 = anchors.item(0);
      expect(anchor1.href).toStrictEqual('https://crbug.com/123');
      expect(anchor1.text).toStrictEqual('123');

      const anchor2 = anchors.item(1);
      expect(anchor2.href).toStrictEqual('https://crbug.com/234');
      expect(anchor2.text).toStrictEqual('234');
    });
  });

  describe('When breaks is set to false', () => {
    const md = MarkdownIt('zero', { breaks: false }).enable('newline').use(bugLine);

    it('can renders multiple bug lines with soft break correctly', async () => {
      const ele = await fixture(md.render(multipleBugLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      expect(anchors.length).toStrictEqual(2);

      const anchor1 = anchors.item(0);
      expect(anchor1.href).toStrictEqual('https://crbug.com/123');
      expect(anchor1.text).toStrictEqual('123');

      const anchor2 = anchors.item(1);
      expect(anchor2.href).toStrictEqual('https://crbug.com/BUG/234');
      expect(anchor2.text).toStrictEqual('BUG:234');
    });

    it('can renders multiple bug lines with hard break correctly', async () => {
      const ele = await fixture(md.render(multipleBugLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      expect(anchors.length).toStrictEqual(2);

      const anchor1 = anchors.item(0);
      expect(anchor1.href).toStrictEqual('https://crbug.com/123');
      expect(anchor1.text).toStrictEqual('123');

      const anchor2 = anchors.item(1);
      expect(anchor2.href).toStrictEqual('https://crbug.com/234');
      expect(anchor2.text).toStrictEqual('234');
    });
  });
});
