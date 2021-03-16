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

import createDomPurify from 'dompurify';
import { AttributeCommitter, DefaultTemplateProcessor, Part, RenderOptions, TemplateResult } from 'lit-html';

const domPurify = createDomPurify(window);

/**
 * A href attribute committer that sanitizes the href attribute.
 * Should only be used to commit href attributes.
 */
class SafeHrefAttributeCommitter extends AttributeCommitter {
  protected _getValue(): string {
    // TODO(crbug/1122567): sanitize the URL directly.
    const value = super._getValue() as string;
    const anchorHtml = domPurify.sanitize(`<a href="${encodeURI(value)}"></a>`);
    const template = document.createElement('template');
    template.innerHTML = anchorHtml;
    return template.content.firstElementChild!.getAttribute('href') || '';
  }
}

/**
 * A processor that sanitizes href attributes.
 * Otherwise identical to DefaultTemplateProcessor.
 */
class SafeHrefTemplateProcessor extends DefaultTemplateProcessor {
  handleAttributeExpressions(
    element: Element,
    name: string,
    strings: string[],
    options: RenderOptions
  ): ReadonlyArray<Part> {
    if (name === 'href') {
      const committer = new SafeHrefAttributeCommitter(element, name, strings);
      return committer.parts;
    }
    return super.handleAttributeExpressions(element, name, strings, options);
  }
}

const safeHrefTemplateProcessor = new SafeHrefTemplateProcessor();

/**
 * Similar to html from 'lit-html', but also sanitizes href attribute.
 */
export const safeHrefHtml = (strings: TemplateStringsArray, ...values: unknown[]) =>
  new TemplateResult(strings, values, 'html', safeHrefTemplateProcessor);
