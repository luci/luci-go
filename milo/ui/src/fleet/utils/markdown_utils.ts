// Copyright 2026 The LUCI Authors.
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

/**
 * Wrapper representing pre-escaped or trusted raw Markdown content that should
 * not be escaped when interpolated inside an `md` tagged template literal.
 */
export class SafeMarkdown extends String {
  constructor(readonly value: string) {
    super();
  }

  toString(): string {
    return this.value;
  }
}

/**
 * Marks a string as trusted/safe Markdown so it will not be escaped when
 * interpolated into `md` tagged templates.
 */
export function rawMd(value: string): SafeMarkdown {
  return new SafeMarkdown(value);
}

/**
 * Escapes Markdown control characters with a backslash to ensure
 * untrusted strings are rendered as literal text rather than formatting constructs
 * (e.g. preventing markdown link injection, arbitrary headers, emphasis, or list items).
 *
 * In accordance with Google Markdown / CommonMark Spec §2.4 (Backslash escapes):
 * https://spec.commonmark.org/current/#backslash-escapes
 * "Any ASCII punctuation character may be backslash-escaped:
 *  ! \" # $ % & ' ( ) * + , - . / : ; < = > ? @ [ \\ ] ^ _ ` { | } ~"
 *
 * Escapes formatting controls, link/image syntax, table pipes, block delimiters,
 * and list markers (\, `, *, _, ~, [, ], (, ), #, |, <, >, {, }, !, +, -, .).
 * Also replaces newlines (\r?\n) with spaces so that untrusted input cannot break
 * out of inline formatting blocks or inject new block-level markdown elements.
 */
export function escapeMarkdown(text?: string): string {
  if (!text) return '';
  return text
    .replace(/\\/g, '\\\\')
    .replace(/([`*_~\[\]()#|<>{}!+\-.])/g, '\\$1')
    .replace(/\r?\n/g, ' ');
}

/**
 * Tagged template literal that safely interpolates dynamic values into Markdown,
 * escaping Markdown control characters and linebreaks in untrusted variables.
 *
 * Usage:
 * ```typescript
 * const line = md`* **Machine:** [${dev.id}](${url})`;
 * ```
 *
 * @param strings Template strings array.
 * @param values Dynamic interpolated expressions.
 * @returns Escaped safe Markdown string.
 */
export function md(
  strings: TemplateStringsArray,
  ...values: readonly unknown[]
): string {
  let result = strings[0];
  for (let i = 0; i < values.length; i++) {
    const val = values[i];
    if (val instanceof SafeMarkdown) {
      result += val.value;
    } else if (val !== undefined && val !== null) {
      result += escapeMarkdown(String(val));
    }
    result += strings[i + 1];
  }
  return result;
}
