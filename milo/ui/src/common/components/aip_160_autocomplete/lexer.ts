// Copyright 2025 The LUCI Authors.
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
 * Enum for token kinds.
 */
export enum TokenKind {
  Whitespace = 'WHITESPACE',
  Paren = 'PAREN',
  Comma = 'COMMA',
  Comparator = 'COMPARATOR',
  Keyword = 'KEYWORD',
  /**
   * `-` with an expression following it.
   */
  Negate = 'NEGATE',
  QualifiedFunction = 'QUALIFIED_FUNCTION',
  QualifiedFieldOrValue = 'QUALIFIED_FIELD_OR_VALUE',
  /**
   * Quoted single line string.
   */
  String = 'STRING',
  /**
   * Quoted single line string but without the closing quote.
   * Terminates before the first line break or at the end of the input.
   */
  UnclosedString = 'UNCLOSED_STRING',
  /**
   * Invalid char. Allow us to process the rest of the tokens even when we
   * encounter an unexpected one.
   */
  InvalidChar = 'INVALID_CHAR',
}

const TOKEN_KINDS = Object.values(TokenKind);

/**
 * A regex that match a quoted string.
 *
 * Contains two cases.
 *  * The first case, /'(?:[^'\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})*'/, matches a
 *    single quoted string.
 *  * The second case matches a double quoted string.
 *
 * /(?:[^'\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})/ matches a character.
 * The character can either be
 *  * any character that is NOT
 *    * a single quote (or a double quote for the double quoted string case),
 *    * a new line
 *    * a backward slash.
 *  * a backward slash with a following character that is not `u` or a newline
 *    (e.g. `\t`).
 *  * a backward slash followed by `u` and four hex digits (e.g. `\u1234`).
 */
const STRING_RE =
  /'(?:[^'\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})*'|"(?:[^"\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})*"/;

/**
 * Similar to STRING_RE but requires the whole string to match STRING_RE.
 */
const SINGLE_STRING_RE = new RegExp(`^(${STRING_RE.source})$`);

/**
 * Similar to STRING_RE except that the closing quote is missing.
 *
 * Matches until a new line or end of string.
 *
 * The lookahead, /(?=\n|$)/, tells the regex to terminate at a newline or end
 * of the string. This prevents a single unclosed quote from consuming the
 * multiple lines when parsing tokens.
 */
const UNCLOSED_STRING_RE =
  /'(?:[^'\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})*(?=\n|$)|"(?:[^"\\\n]|\\[^u\n]|\\u[0-9a-fA-F]{4})*(?=\n|$)/;

/**
 * Similar to UNCLOSED_STRING_RE but requires the whole string to match
 * UNCLOSED_STRING_RE.
 */
const SINGLE_UNCLOSED_STRING_RE = new RegExp(
  `^(${UNCLOSED_STRING_RE.source})$`,
);

/**
 * LEXER_RE has one capture group for each kind of token that can be lexed.
 * The ith capturing group represents a token of the ith kind in `TokenKind`.
 */
const LEXER_RE = new RegExp(
  /^/.source +
    [
      // Whitespace
      /\s+/,
      // Paren
      /[()]/,
      // Comma
      /,/,
      // Comparator
      /<|<=|=|>=|>|!=|:/,
      // Keyword
      /AND\b|OR\b|NOT\b/,
      // Negate
      /-(?!\s)/,
      // QualifiedFunction
      /[\w._-]+\(/,
      // QualifiedField
      /[\w._-]+/,
      // String
      STRING_RE,
      // UnclosedString
      UNCLOSED_STRING_RE,
      // InvalidChar
      /.|\n/,
    ]
      .map((r) => `(${r.source})`)
      .join('|'),
);

export interface Token {
  readonly index: number;
  readonly startPos: number;
  readonly kind: TokenKind;
  readonly text: string;
}

export class Lexer {
  private cachedTokens: Token[] = [];
  private unprocessedInput: string;
  private processedLen = 0;

  constructor(private input: string) {
    this.unprocessedInput = this.input;
  }

  getAllTokens(): readonly Token[] {
    while (this.unprocessedInput !== '') {
      this.processNextToken();
    }
    return this.cachedTokens;
  }

  /**
   * Get token(s) at the specified cursor position. Return
   *  * two tokens: if the cursor divide two tokens (e.g. `text|<=`).
   *  * one token: if the cursor is in the middle of a token (e.g. `te|xt<=`)
   *  * zero token: if the cursor is not placed on any token (e.g. `text<=  |`)
   */
  getAtPosition(
    cursorPos: number,
  ): readonly [Token, Token] | readonly [Token] | readonly [] {
    while (this.unprocessedInput !== '' && this.processedLen <= cursorPos + 1) {
      this.processNextToken();
    }

    // The text is empty.
    if (this.processedLen === 0) {
      return [];
    }

    // The cursor position is not in the range of the text [0, text_length].
    if (cursorPos < 0 || cursorPos > this.processedLen) {
      return [];
    }

    // Find the LAST token that starts before the cursor.
    let tokenIndex = -1;
    for (let i = 0; i < this.cachedTokens.length; ++i) {
      if (this.cachedTokens[i].startPos >= cursorPos) {
        break;
      }
      tokenIndex = i;
    }

    // `prevToken !== null` is always true because the previous two early
    // returns guarantee there's a token that starts before the cursor.
    const prevToken = this.cachedTokens[tokenIndex];
    const nextToken = this.cachedTokens[tokenIndex + 1] as Token | undefined;

    // If the next token starts at the cursor position, the cursor must divide
    // two tokens (e.g. `${prevToken}|${nextToken}`). Return both of them.
    if (nextToken?.startPos === cursorPos) {
      return [prevToken, nextToken];
    }

    return [prevToken];
  }

  /**
   * Get the last token with an index smaller than the specified index.
   * Useful for retrieving the previous token of a given token (index).
   *
   * @param allowWS When set the true, include `Whitespace` token. Defaults to
   *   false.
   */
  getBeforeIndex(beforeIndex: number, allowWS = false): Token | null {
    while (
      this.unprocessedInput !== '' &&
      this.cachedTokens.length < beforeIndex
    ) {
      this.processNextToken();
    }

    const searchStart = Math.min(beforeIndex - 1, this.cachedTokens.length - 1);
    for (let i = searchStart; i >= 0; --i) {
      const token = this.cachedTokens[i];
      if (!allowWS && token.kind === TokenKind.Whitespace) {
        continue;
      }
      return token;
    }
    return null;
  }

  /**
   * Get the first token with an index greater than the specified index.
   * Useful for retrieving the next token of a given token (index).
   *
   * @param allowWS When set the true, include `Whitespace` token. Defaults to
   *   false.
   */
  getAfterIndex(afterIndex: number, allowWS = false): Token | null {
    const searchStart = Math.max(afterIndex + 1, 0);
    let i = searchStart;
    for (;;) {
      while (i >= this.cachedTokens.length && this.unprocessedInput !== '') {
        this.processNextToken();
      }
      if (i >= this.cachedTokens.length) {
        return null;
      }

      const token = this.cachedTokens[i];
      ++i;
      if (!allowWS && token.kind === TokenKind.Whitespace) {
        continue;
      }
      return token;
    }
  }

  /**
   * Process the next token and append it to `this.cachedTokens`.
   */
  private processNextToken() {
    if (this.unprocessedInput === '') {
      return;
    }

    // Match the next token string.
    const matches = LEXER_RE.exec(this.unprocessedInput);
    if (!matches) {
      throw new Error(
        `invariant violated: unable to lex token from '${this.input}' at position ${this.processedLen}`,
      );
    }
    const startPos = this.processedLen;
    this.processedLen += matches[0].length;
    this.unprocessedInput = this.unprocessedInput.slice(matches[0].length);

    // Map the token string to a token kind.
    const kindIter = TOKEN_KINDS.entries();
    for (const [i, kind] of kindIter) {
      if (matches[i + 1] === undefined) {
        continue;
      }
      this.cachedTokens.push({
        index: this.cachedTokens.length,
        startPos,
        kind,
        text: matches[i + 1],
      });
      return;
    }

    // Should never happen because we have a catch all token type.
    throw new Error(
      `invariant violated: unhandled lexer regexp match '${matches[TOKEN_KINDS.length]}'`,
    );
  }
}

/**
 * Try to unquote a string.
 *
 * 1. If it's partially quoted, make it fully quoted.
 * 2. If it's quoted, unquote it.
 * 3. If it's still not a valid quoted string after the fix, return the original
 *    string.
 */
export function tryUnquoteStr(input: string) {
  let fixed = input;
  // If the string quoted but unclosed, close it first.
  if (SINGLE_UNCLOSED_STRING_RE.test(fixed)) {
    fixed += fixed[0];
  }

  // If the string is quoted, try unquote it.
  if (SINGLE_STRING_RE.test(fixed)) {
    // Convert single quoted string to double quoted string.
    if (fixed[0] === "'") {
      const inner = fixed.slice(1, -1);
      const escapedDoubleQuote = inner.replace(
        // Only match double quotes that have an even number of leading \.
        // Double quotes with an odd number of leading \ are already escaped.
        /(?<!\\)(?:\\\\)*"/g,
        (partial) => partial.replace('"', '\\"'),
      );
      fixed = `"${escapedDoubleQuote}"`;
    }

    try {
      return JSON.parse(fixed);
    } catch (_) {
      // Do nothing.
    }
  }

  return input;
}
