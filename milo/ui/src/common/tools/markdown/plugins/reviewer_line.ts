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

import MarkdownIt from 'markdown-it';
import Token from 'markdown-it/lib/token.mjs';

import { specialLine } from './special_line';

/**
 * Support reviewer line (e.g. 'R=user@google.com',
 * 'r=user2@google.com,user2@google.com').
 *
 * This prevents the 'r=' prefix being treated as a part of the email address.
 */
export function reviewerLine(md: MarkdownIt) {
  // specialLine will extract R= to a separate text node, which prevents R=
  // being treated as part of an email address.
  // We don't need to do any transformation.
  md.use(specialLine, /^r=/i, (token: Token) => [token]);
}
