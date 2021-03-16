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
import MarkdownIt from 'markdown-it';

import { bugLine } from './bug_line';

const singleBugLine = 'Bug: 123, 234, not a project, proj-1:345';
const multipleBugLinesWithSoftBreak = 'Bug: 123\nBUG:234';
const multipleBugLinesWithHardBreak = 'Bug: 123  \nBUG:234';

describe('bug_line', () => {
  it('can render single bug line correctly', async () => {
    const md = MarkdownIt('zero', { breaks: true }).enable('newline').use(bugLine);

    after(fixtureCleanup);
    const ele = await fixture(md.render(singleBugLine));

    const anchors = ele.querySelectorAll('a');
    assert.equal(anchors.length, 3);

    const anchor1 = anchors.item(0);
    assert.equal(anchor1.href, 'https://crbug.com/123');
    assert.equal(anchor1.text, '123');

    const anchor2 = anchors.item(1);
    assert.equal(anchor2.href, 'https://crbug.com/234');
    assert.equal(anchor2.text, '234');

    const anchor3 = anchors.item(2);
    assert.equal(anchor3.href, 'https://crbug.com/proj-1/345');
    assert.equal(anchor3.text, 'proj-1:345');

    assert.equal(ele.childNodes[4].textContent, ', not a project, ');
  });

  describe('When breaks is set to true', () => {
    const md = MarkdownIt('zero', { breaks: true }).enable('newline').use(bugLine);

    it('can renders multiple bug lines with soft break correctly', async () => {
      after(fixtureCleanup);
      const ele = await fixture(md.render(multipleBugLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'https://crbug.com/123');
      assert.equal(anchor1.text, '123');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'https://crbug.com/234');
      assert.equal(anchor2.text, '234');
    });

    it('can renders multiple bug lines with hard break correctly', async () => {
      after(fixtureCleanup);
      const ele = await fixture(md.render(multipleBugLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'https://crbug.com/123');
      assert.equal(anchor1.text, '123');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'https://crbug.com/234');
      assert.equal(anchor2.text, '234');
    });
  });

  describe('When breaks is set to false', () => {
    const md = MarkdownIt('zero', { breaks: false }).enable('newline').use(bugLine);

    it('can renders multiple bug lines with soft break correctly', async () => {
      after(fixtureCleanup);
      const ele = await fixture(md.render(multipleBugLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'https://crbug.com/123');
      assert.equal(anchor1.text, '123');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'https://crbug.com/BUG/234');
      assert.equal(anchor2.text, 'BUG:234');
    });

    it('can renders multiple bug lines with hard break correctly', async () => {
      after(fixtureCleanup);
      const ele = await fixture(md.render(multipleBugLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'https://crbug.com/123');
      assert.equal(anchor1.text, '123');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'https://crbug.com/234');
      assert.equal(anchor2.text, '234');
    });
  });
});
