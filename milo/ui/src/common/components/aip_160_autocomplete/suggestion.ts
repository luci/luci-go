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

import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { OptionDef } from '@/generic_libs/components/text_autocomplete';

import { Lexer, Token, TokenKind } from './lexer';
import { FieldDef, FieldsSchema, Suggestion } from './types';

export function useSuggestions(
  schema: FieldsSchema,
  input: string,
  cursorPos: number,
): readonly OptionDef<Suggestion>[] {
  const lexer = useMemo(() => new Lexer(input), [input]);
  const ctx = getSuggestionCtx(lexer, cursorPos);

  const optionsSync = useSuggestionsSync(schema, input, ctx);
  const optionsAsync = useSuggestionsAsync(schema, input, ctx);

  return [...optionsSync, ...optionsAsync];
}

/**
 * Token selection priority. Lower index -> higher priority.
 *
 * When the cursor is between two tokens, the one with higher priority will be
 * selected.
 */
const TOKEN_SELECTION_PRIORITY = Object.freeze([
  TokenKind.QualifiedFieldOrValue,
  TokenKind.String,
  TokenKind.UnclosedString,
  TokenKind.QualifiedFunction,
  TokenKind.Comparator,
  TokenKind.Paren,
  TokenKind.Comma,
  TokenKind.Keyword,
  TokenKind.Negate,
  TokenKind.InvalidChar,
  TokenKind.Whitespace,
  undefined,
]);

export type SuggestionContext =
  | SuggestionContextField
  | SuggestionContextValue
  | null;

export interface SuggestionContextField {
  readonly type: 'field';
  readonly fieldToken: Token;
}

export interface SuggestionContextValue {
  readonly type: 'value';
  readonly fieldToken: Token;
  readonly valueToken: Token;
}

export function getSuggestionCtx(
  lexer: Lexer,
  cursorPos: number,
): SuggestionContext {
  const tokens = lexer.getAtPosition(cursorPos);

  // Get the token at the cursor position.
  // When the cursor is between two tokens, select the one with higher priority.
  const selectedToken = [...tokens, undefined].sort(
    (a, b) =>
      TOKEN_SELECTION_PRIORITY.indexOf(a?.kind) -
      TOKEN_SELECTION_PRIORITY.indexOf(b?.kind),
  )[0];

  if (!selectedToken) {
    return null;
  }

  const prevToken1 = lexer.getBeforeIndex(selectedToken.index);
  const prevToken2 = prevToken1 ? lexer.getBeforeIndex(prevToken1.index) : null;

  // Suggest value if the current token can be interpreted as a value token.

  // Example: `field > val|ue`
  if (
    prevToken2?.kind === TokenKind.QualifiedFieldOrValue &&
    prevToken1?.kind === TokenKind.Comparator &&
    selectedToken.kind === TokenKind.QualifiedFieldOrValue
  ) {
    return {
      type: 'value',
      fieldToken: prevToken2,
      valueToken: selectedToken,
    };
  }

  // Example: `field > "val|ue"`
  if (
    prevToken2?.kind === TokenKind.QualifiedFieldOrValue &&
    prevToken1?.kind === TokenKind.Comparator &&
    selectedToken.kind === TokenKind.String
  ) {
    return {
      type: 'value',
      fieldToken: prevToken2,
      valueToken: selectedToken,
    };
  }

  // Example: `field > "val|ue`
  if (
    prevToken2?.kind === TokenKind.QualifiedFieldOrValue &&
    prevToken1?.kind === TokenKind.Comparator &&
    selectedToken.kind === TokenKind.UnclosedString
  ) {
    return {
      type: 'value',
      fieldToken: prevToken2,
      valueToken: selectedToken,
    };
  }

  // Example: `field >|`
  if (
    prevToken1?.kind === TokenKind.QualifiedFieldOrValue &&
    selectedToken.kind === TokenKind.Comparator
  ) {
    return {
      type: 'value',
      fieldToken: prevToken1,
      valueToken: {
        index: selectedToken.index + 1,
        startPos: selectedToken.startPos + selectedToken.text.length,
        kind: TokenKind.QualifiedFieldOrValue,
        text: '',
      },
    };
  }

  // Example: `field >  | `
  if (
    prevToken2?.kind === TokenKind.QualifiedFieldOrValue &&
    prevToken1?.kind === TokenKind.Comparator &&
    selectedToken.kind === TokenKind.Whitespace
  ) {
    return {
      type: 'value',
      fieldToken: prevToken2,
      valueToken: {
        index: selectedToken.index,
        // Leave one space no matter how long the whitespace is.
        startPos: selectedToken.startPos + 1,
        kind: TokenKind.QualifiedFieldOrValue,
        text: '',
      },
    };
  }

  // Suggest field otherwise.

  // Example: fie|ld
  if (selectedToken.kind === TokenKind.QualifiedFieldOrValue) {
    return {
      type: 'field',
      fieldToken: selectedToken,
    };
  }

  return null;
}

function useSuggestionsSync(
  schema: FieldsSchema,
  input: string,
  ctx: SuggestionContext,
): readonly OptionDef<Suggestion>[] {
  const options: OptionDef<Suggestion>[] = [];
  if (ctx?.type === 'value') {
    options.push(
      ...suggestValue(schema, input, ctx.fieldToken, ctx.valueToken),
    );
  }
  if (ctx?.type === 'field') {
    options.push(...suggestField(schema, input, ctx.fieldToken));
  }
  return options;
}

function useSuggestionsAsync(
  schema: FieldsSchema,
  input: string,
  ctx: SuggestionContext,
): readonly OptionDef<Suggestion>[] {
  const valueQuery = (ctx &&
    ctx.type === 'value' &&
    getField(schema, ctx.fieldToken.text)?.fetchValues?.(
      ctx.valueToken.text,
    )) || {
    queryKey: ['always-disabled'],
    queryFn: () => [],
    enabled: false,
  };

  const { data, isError, error, isLoading, isFetching } = useQuery(valueQuery);
  if (isError) {
    throw error;
  }

  if (isLoading && !isFetching) {
    return [];
  }
  // This is already covered by the case above. Add it for type narrowing.
  if (ctx?.type !== 'value') {
    return [];
  }

  if (isLoading) {
    return [
      {
        id: 'loading',
        value: {
          text: 'Loading...',
          // Doesn't matter. This option is not selectable.
          apply: () => ['', 0] as const,
        },
        unselectable: true,
      },
    ];
  }

  return (
    data?.map((valueDef) => ({
      id: valueDef.text,
      value: {
        text: valueDef.text,
        apply: () => {
          const prefixEnd = ctx.valueToken.startPos;
          const suffixStart =
            ctx.valueToken.startPos + ctx.valueToken.text.length;
          return [
            input.slice(0, prefixEnd) +
              valueDef.text +
              input.slice(suffixStart),
            prefixEnd + valueDef.text.length,
          ] as const;
        },
      },
    })) || []
  );
}

function suggestField(
  schema: FieldsSchema,
  input: string,
  fieldToken: Token,
): readonly OptionDef<Suggestion>[] {
  const lastDotIndex = fieldToken.text.lastIndexOf('.');
  const prev = fieldToken.text.slice(0, Math.max(lastDotIndex, 0));
  const suffix = fieldToken.text.slice(lastDotIndex + 1);
  const suffixLower = suffix.toLowerCase();
  const targetSchema =
    prev === '' ? schema : getField(schema, prev)?.fields || {};

  return Object.keys(targetSchema)
    .filter(
      (name) => name.toLowerCase().includes(suffixLower) && name !== suffix,
    )
    .map((name) => ({
      id: name,
      value: {
        text: name,
        apply: () => {
          const prefixEnd = fieldToken.startPos + lastDotIndex + 1;
          const suffixStart =
            prefixEnd + fieldToken.text.length - lastDotIndex - 1;
          return [
            input.slice(0, prefixEnd) + name + input.slice(suffixStart),
            prefixEnd + name.length,
          ] as const;
        },
      },
    }));
}

function suggestValue(
  schema: FieldsSchema,
  input: string,
  fieldToken: Token,
  valueToken: Token,
): readonly OptionDef<Suggestion>[] {
  const field = getField(schema, fieldToken.text);
  return (
    field?.getValues?.(valueToken.text).map((valueDef) => ({
      id: valueDef.text,
      value: {
        text: valueDef.text,
        apply: () => {
          const prefixEnd = valueToken.startPos;
          const suffixStart = valueToken.startPos + valueToken.text.length;
          return [
            input.slice(0, prefixEnd) +
              valueDef.text +
              input.slice(suffixStart),
            prefixEnd + valueDef.text.length,
          ] as const;
        },
      },
    })) || []
  );
}

function getField(schema: FieldsSchema, fieldPath: string): FieldDef | null {
  const fieldNames = fieldPath.split('.');

  let pivot = schema;
  for (let i = 0; i < fieldNames.length - 1; ++i) {
    const fieldName = fieldNames[i];
    const fieldDef = schema[fieldName];
    if (!fieldDef?.fields) {
      return null;
    }
    pivot = fieldDef.fields;
  }
  return pivot[fieldNames.at(-1)!] ?? null;
}
