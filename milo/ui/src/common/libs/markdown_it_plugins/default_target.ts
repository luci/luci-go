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

const CLOSING_ANCHOR_TAG = '</a>';

/**
 * Set default target for link elements.
 */
export function defaultTarget(md: MarkdownIt, defaultTarget: string) {
  const renderToken = md.renderer.renderToken.bind(md.renderer);

  // Set default target for link_open tokens.
  const existingLinkOpenRule = md.renderer.rules['link_open'] || renderToken;
  md.renderer.rules['link_open'] = (tokens, i, ...params) => {
    const target = tokens[i].attrGet('target') || defaultTarget;
    tokens[i].attrSet('target', target);
    return existingLinkOpenRule(tokens, i, ...params);
  };

  // Set default target for anchors in html_inline tokens.
  const existingHTMLInlineRule =
    md.renderer.rules['html_inline'] || renderToken;
  md.renderer.rules['html_inline'] = (tokens, i, ...params) => {
    const token = tokens[i];
    if (/^<a .*>$/i.test(token.content)) {
      const template = document.createElement('template');
      template.innerHTML = token.content + CLOSING_ANCHOR_TAG;
      const anchor = template.content.firstElementChild!;
      const target = anchor.getAttribute('target') || defaultTarget;
      anchor.setAttribute('target', target);
      token.content = anchor.outerHTML.substring(
        0,
        anchor.outerHTML.length - CLOSING_ANCHOR_TAG.length
      );
    }
    return existingHTMLInlineRule(tokens, i, ...params);
  };

  // Set default target for anchors in html_block tokens.
  const existingHTMLBlockRule = md.renderer.rules['html_block'] || renderToken;
  md.renderer.rules['html_block'] = (tokens, i, ...params) => {
    const token = tokens[i];
    const template = document.createElement('template');
    template.innerHTML = token.content;
    const anchors = template.content.querySelectorAll('a');
    anchors.forEach((anchor) => {
      const target = anchor.getAttribute('target') || defaultTarget;
      anchor.setAttribute('target', target);
    });
    token.content = template.innerHTML;
    return existingHTMLBlockRule(tokens, i, ...params);
  };
}
