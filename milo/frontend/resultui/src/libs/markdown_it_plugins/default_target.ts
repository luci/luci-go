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

/**
 * Set default target for link_open elements.
 */
export function defaultTarget(md: MarkdownIt, defaultTarget: string) {
  const existingRule = (md.renderer.rules['link_open'] ||
    md.renderer.renderToken).bind(md.renderer);

  md.renderer.rules['link_open'] = (tokens, i, ...params) => {
    const target = tokens[i].attrGet('target') || defaultTarget;
    tokens[i].attrSet('target', target);
    return existingRule(tokens, i, ...params);
  };
}
