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

import { html, TemplateResult } from 'lit';

/**
 * Return a lit-element template that highlight the substring (case-insensitive)
 * in the given fullString.
 */
export function highlight(
  fullString: string,
  subString: string,
): TemplateResult {
  const matchStart = fullString.toUpperCase().search(subString.toUpperCase());
  const prefix = fullString.slice(0, matchStart);
  const matched = fullString.slice(matchStart, matchStart + subString.length);
  const suffix = fullString.slice(matchStart + subString.length);
  return html`${prefix}<strong>${matched}</strong>${suffix}`;
}
