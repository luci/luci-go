// Copyright 2021 The LUCI Authors.
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
import Token from 'markdown-it/lib/token.mjs';

import { specialLine } from './special_line';

function lookaheadLine(md: markdownIt) {
  md.use(specialLine, /^(?=abc=)/i, (token: Token) => {
    token.content = 'special';
    return [token];
  });
}

describe('special_line', () => {
  test('can handle regex matching empty prefix (e.g. lookahead regex)', async () => {
    const md = markdownIt('zero', { breaks: true, linkify: true }).use(
      lookaheadLine,
    );

    const ele = await fixture(md.render('abc=content'));
    expect(ele.textContent).toStrictEqual('special');
  });
});
