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

import { CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { HighlightedText } from '@/generic_libs/components/highlighted_text';
import { OptionDef } from '@/generic_libs/components/text_autocomplete';

import { Lexer, Token, TokenKind } from './lexer';
import { FieldDef, Suggestion } from './types';

export function useSuggestions(
  schema: FieldDef,
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
  TokenKind.InvalidString,
  TokenKind.UnclosedInvalidString,
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
  schema: FieldDef,
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
  schema: FieldDef,
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
          display: (
            <b>
              Loading <CircularProgress size="1em" />
            </b>
          ),
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
      unselectable: valueDef.unselectable,
      value: {
        text: valueDef.text,
        display: valueDef.display,
        explanation: valueDef.explanation,
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
  schema: FieldDef,
  input: string,
  fieldToken: Token,
): readonly OptionDef<Suggestion>[] {
  const [targetSchema, suffix] = probField(
    schema,
    fieldToken.text,
    fieldToken.text.match(/\./g)?.length ?? 0,
  );
  const prefixEnd =
    fieldToken.startPos + fieldToken.text.length - suffix.length;
  const suffixStart = prefixEnd + fieldToken.text.length;

  const suffixLower = suffix.toLowerCase();
  const staticFieldOptions = suffix.includes('.')
    ? []
    : Object.keys(targetSchema?.staticFields || {})
        .filter(
          (name) => name.toLowerCase().includes(suffixLower) && name !== suffix,
        )
        .map<OptionDef<Suggestion>>((name) => ({
          id: name,
          value: {
            text: name,
            display: <HighlightedText text={name} highlight={suffix} />,
            apply: () => [
              input.slice(0, prefixEnd) + name + input.slice(suffixStart),
              prefixEnd + name.length,
            ],
          },
        }));

  const dynamicFieldOptions =
    targetSchema.dynamicFields
      ?.getKeys?.(suffix)
      .map<OptionDef<Suggestion>>((valueDef) => ({
        id: valueDef.text,
        unselectable: valueDef.unselectable,
        value: {
          text: valueDef.text,
          display: valueDef.display,
          explanation: valueDef.explanation,
          apply: () => [
            input.slice(0, prefixEnd) +
              valueDef.text +
              input.slice(suffixStart),
            prefixEnd + valueDef.text.length,
          ],
        },
      })) || [];

  return [...staticFieldOptions, ...dynamicFieldOptions];
}

function suggestValue(
  schema: FieldDef,
  input: string,
  fieldToken: Token,
  valueToken: Token,
): readonly OptionDef<Suggestion>[] {
  const [field, suffix] = probField(schema, fieldToken.text);
  const prefixEnd = valueToken.startPos;
  const suffixStart = valueToken.startPos + valueToken.text.length;

  // Suggest value for a statically known field.
  if (suffix === '') {
    return (
      field
        ?.getValues?.(valueToken.text)
        .map<OptionDef<Suggestion>>((valueDef) => ({
          id: valueDef.text,
          unselectable: valueDef.unselectable,
          value: {
            text: valueDef.text,
            display: valueDef.display,
            explanation: valueDef.explanation,
            apply: () => {
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

  // Suggest value for a dynamic field.
  return (
    field?.dynamicFields
      ?.getValues?.(suffix, valueToken.text)
      .map<OptionDef<Suggestion>>((valueDef) => ({
        id: valueDef.text,
        unselectable: valueDef.unselectable,
        value: {
          text: valueDef.text,
          display: valueDef.display,
          explanation: valueDef.explanation,
          apply: () => [
            input.slice(0, prefixEnd) +
              valueDef.text +
              input.slice(suffixStart),
            prefixEnd + valueDef.text.length,
          ],
        },
      })) || []
  );
}

function getField(schema: FieldDef, fieldPath: string): FieldDef | null {
  const [field, remaining] = probField(schema, fieldPath);
  if (remaining) {
    return null;
  }
  return field;
}

/**
 * Get the last field when traversing the field def via the specified
 * `fieldPath`.
 *
 * Also return the remaining field path segments that were not traversed due to
 * unmatched field names or reaching maximum depth limit.
 */
function probField(
  schema: FieldDef,
  fieldPath: string,
  maxDepth = Infinity,
): readonly [FieldDef, string] {
  const fieldNames = fieldPath.split('.');
  const limit = Math.min(fieldNames.length, maxDepth);

  let pivot = schema;
  let visitedCount = 0;
  for (let i = 0; i < limit; ++i) {
    const fieldName = fieldNames[i];
    const fieldDef = pivot.staticFields?.[fieldName];
    if (!fieldDef) {
      break;
    }
    pivot = fieldDef;
    visitedCount = i + 1;
  }
  return [pivot, fieldNames.slice(visitedCount).join('.')];
}
