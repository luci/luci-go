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

import { reviewerLine } from './reviewer_line';

const singleReviewerLine = 'R=user@google.com';
const multipleReviewerLinesWithSoftBreak = 'R=user@google.com\nR=user2@google.com';
const multipleReviewerLinesWithHardBreak = 'R=user@google.com  \nR=user2@google.com';

describe('reviewer_line', () => {
  it('does not treat "R=" prefix as part of the email address', async () => {
    const md = MarkdownIt('zero', { breaks: true, linkify: true }).enable(['newline', 'linkify']).use(reviewerLine);

    const ele = await fixture(md.render(singleReviewerLine));

    const anchors = ele.querySelectorAll('a');
    assert.equal(anchors.length, 1);
    const anchor1 = anchors.item(0);
    assert.equal(anchor1.href, 'mailto:user@google.com');
    assert.equal(anchor1.text, 'user@google.com');
  });

  describe('When breaks is set to true', () => {
    const md = MarkdownIt('zero', { breaks: true, linkify: true }).enable(['newline', 'linkify']).use(reviewerLine);

    it('can renders multiple reviewer lines with soft break correctly', async () => {
      const ele = await fixture(md.render(multipleReviewerLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'mailto:user@google.com');
      assert.equal(anchor1.text, 'user@google.com');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'mailto:user2@google.com');
      assert.equal(anchor2.text, 'user2@google.com');
    });

    it('can renders multiple reviewer lines with hard break correctly', async () => {
      const ele = await fixture(md.render(multipleReviewerLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'mailto:user@google.com');
      assert.equal(anchor1.text, 'user@google.com');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'mailto:user2@google.com');
      assert.equal(anchor2.text, 'user2@google.com');
    });
  });

  describe('When breaks is set to false', () => {
    const md = MarkdownIt('zero', { breaks: false, linkify: true }).enable(['newline', 'linkify']).use(reviewerLine);

    it('can renders multiple reviewer lines with soft break correctly', async () => {
      const ele = await fixture(md.render(multipleReviewerLinesWithSoftBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'mailto:user@google.com');
      assert.equal(anchor1.text, 'user@google.com');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'mailto:R=user2@google.com');
      assert.equal(anchor2.text, 'R=user2@google.com');
    });

    it('can renders multiple reviewer lines with hard break correctly', async () => {
      const ele = await fixture(md.render(multipleReviewerLinesWithHardBreak));

      const anchors = ele.querySelectorAll('a');
      assert.equal(anchors.length, 2);

      const anchor1 = anchors.item(0);
      assert.equal(anchor1.href, 'mailto:user@google.com');
      assert.equal(anchor1.text, 'user@google.com');

      const anchor2 = anchors.item(1);
      assert.equal(anchor2.href, 'mailto:user2@google.com');
      assert.equal(anchor2.text, 'user2@google.com');
    });
  });
});
