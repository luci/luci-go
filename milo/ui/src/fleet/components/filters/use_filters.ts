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

import { useCallback, useMemo } from 'react';

import * as ast from '@/fleet/utils/aip160/ast/ast';
import { parseFilter } from '@/fleet/utils/aip160/parser/parser';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { FILTERS_PARAM_KEY } from '../filter_dropdown/search_param_utils';

import { LoadingFilterCategory } from './loading_filter';

type FilterValuesFromBuilders<T> = {
  [Key in keyof T]: T[Key] extends FilterCategoryBuilder<infer FC> ? FC : never;
};

export const useFilters = <
  T extends Record<string, FilterCategoryBuilder<FilterCategory>>,
>(
  rawBuilders: T | undefined,
  options = { allowExtraKeys: false },
): {
  filterValues: FilterValuesFromBuilders<T> | undefined;
  aip160: string;
  parseError: string | undefined;
  getAip160String: () => string;
} => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const filtersAIP160 = searchParams.get(FILTERS_PARAM_KEY);

  // Stabilize builders to prevent infinite loops if the parent passes a new object every render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const builders = useMemo(() => rawBuilders, [JSON.stringify(rawBuilders)]);

  const updateUrl = useCallback(
    function onFilterUpdate(
      filterValues: Record<string, FilterCategory>,
      replaceHistory: boolean = false,
    ): void {
      setSearchParams(
        (prev: URLSearchParams) => {
          const filter = Object.values(filterValues)
            .map((f) => f.toAIP160())
            .filter((f) => f !== '')
            .join(' AND ');

          prev.set(FILTERS_PARAM_KEY, filter);
          return prev;
        },
        { replace: replaceHistory },
      );
    },
    [setSearchParams],
  );

  const [filterValues, parseError]: [
    FilterValuesFromBuilders<T> | undefined,
    string | undefined,
  ] = useMemo(() => {
    if (builders === undefined) return [undefined, undefined];

    const { filters, parseError } = buildFilters(
      builders,
      updateUrl,
      filtersAIP160,
      options.allowExtraKeys,
    );
    if (
      filters !== undefined &&
      parseError === undefined &&
      !Object.values(filters).some((o) => o instanceof LoadingFilterCategory)
    ) {
      updateUrl(filters, true);
    }

    return [filters, parseError];
  }, [builders, filtersAIP160, updateUrl, options.allowExtraKeys]);

  const getAip160String = useCallback(() => {
    if (!filterValues) return '';
    return Object.values(filterValues)
      .filter((f) => f.isActive())
      .map((f) => f.toAIP160())
      .filter((f) => f !== '')
      .join(' AND ');
  }, [filterValues]);

  return {
    filterValues: filterValues,
    aip160: filtersAIP160 ?? '',
    parseError,
    getAip160String,
  };
};

export interface FilterCategoryBuilder<T extends FilterCategory> {
  isFilledIn(): boolean;
  build(
    key: string,
    reRender: (newFilter: T) => void,
    // If the url is missing FILTERS_PARAM_KEY terms will equal null
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ): T;
}

export interface FilterCategory {
  key: string;
  label: string;

  toAIP160(): string;
  render(
    childrenSearchQuery: string,
    onNavigateUp: (e: React.KeyboardEvent) => void,
    onApply: () => void,
    onClose: () => void,
    ref?: React.Ref<unknown>,
  ): React.ReactNode;
  getChipLabel: () => string;
  isActive: () => boolean;
  clear: () => void;
  getChildrenSearchScore: (searchQuery: string) => number;
  setReRender: (reRender: (newFilter: FilterCategory) => void) => void;
}

type ParseResult =
  | {
      isError: false;
      terms: Record<string, (ast.Term & { simple: ast.Restriction })[]>;
    }
  | { isError: true; error: string };

const constructFiltersFromAIP160 = (filtersAIP160: string): ParseResult => {
  const wrapper = (node: ast.Node | null): ParseResult => {
    if (!node) return { isError: false, terms: {} };

    switch (node.kind) {
      case 'Filter':
        return node.expression
          ? wrapper(node.expression)
          : { isError: false, terms: {} };
      case 'Expression':
        return node.sequences.reduce(
          (acc, seq) => {
            if (acc.isError) {
              return acc;
            }

            const restrictions = wrapper(seq);
            if (restrictions.isError) {
              return restrictions;
            }
            for (const [key, value] of Object.entries(restrictions.terms)) {
              if (!acc.terms[key]) {
                acc.terms[key] = [];
              }
              acc.terms[key].push(...value);
            }
            return { isError: false, terms: acc.terms };
          },
          { isError: false, terms: {} } as ParseResult,
        );
      case 'Sequence':
        return node.factors.reduce(
          (acc, seq) => {
            if (acc.isError) {
              return acc;
            }

            const restrictions = wrapper(seq);
            if (restrictions.isError) {
              return restrictions;
            }
            for (const [key, value] of Object.entries(restrictions.terms)) {
              if (!acc.terms[key]) {
                acc.terms[key] = [];
              }
              acc.terms[key].push(...value);
            }
            return { isError: false, terms: acc.terms };
          },
          { isError: false, terms: {} } as ParseResult,
        );
      case 'Factor':
        const wrappedTerms = combineWrappedResults(node.terms.map(wrapper));
        if (wrappedTerms.isError) return wrappedTerms;

        if (Object.keys(wrappedTerms.terms).length > 1)
          return {
            isError: true,
            error: 'OR between filters is not supported yet',
          };

        return wrappedTerms;
      case 'Term':
        if (node.simple.kind === 'Restriction') {
          return {
            isError: false,
            terms: {
              [memberToKey(node.simple.comparable.member)]: [
                {
                  kind: 'Term',
                  negated: node.negated,
                  simple: node.simple,
                },
              ],
            },
          };
        }

        if (node.negated) {
          return {
            isError: true,
            error: 'NOT (...) terms are not supported yet',
          };
        }

        return wrapper(node.simple);
      case 'Restriction':
      case 'Comparable':
      case 'Member':
      case 'Value':
        return {
          isError: true,
          error: 'Something went wrong while parsing the filters.',
        };
    }
  };

  const parseResult = parseFilter(filtersAIP160);
  if (parseResult.isError) {
    return { isError: true, error: parseResult.error };
  }

  return wrapper(parseResult.ast);
};

export const memberToKey = (node: ast.Member): string => {
  const key = node.value.quoted ? `"${node.value.value}"` : node.value.value;

  return [
    key,
    ...node.fields.map((f) => (f.quoted ? `"${f.value}"` : f.value)),
  ].join('.');
};
function buildFilters<
  T extends Record<string, FilterCategoryBuilder<FilterCategory>>,
>(
  builders: T,
  // onFilterUpdate: (key: string, newFilterValue: FilterCategory) => void,
  onFilterUpdate: (filterValues: Record<string, FilterCategory>) => void,
  filtersAIP160: string | null,
  allowExtraKeys: boolean,
): {
  filters: FilterValuesFromBuilders<T> | undefined;
  parseError: string | undefined;
} {
  const parseResult =
    filtersAIP160 === null ? null : constructFiltersFromAIP160(filtersAIP160);

  if (parseResult?.isError) {
    return { filters: undefined, parseError: parseResult.error };
  }

  const filters: Record<string, FilterCategory> = {};
  const parseErrors: string[] = [];

  for (const [key, bob] of Object.entries(builders)) {
    if (!bob.isFilledIn()) {
      throw new Error(`Builder ${key} is not filled in: ${bob}`);
    }

    const terms = parseResult === null ? null : (parseResult.terms[key] ?? []);
    try {
      filters[key] = bob.build(key, () => {}, terms);
    } catch (e) {
      parseErrors.push(
        `Error with ${key}: ${e instanceof Error ? e.message : String(e)}`,
      );
    }
  }

  if (parseResult !== null) {
    for (const [key, _terms] of Object.entries(parseResult.terms)) {
      if (!filters[key]) {
        if (allowExtraKeys) {
          filters[key] = new LoadingFilterCategory(key);
        } else {
          parseErrors.push(`${key} is not a valid filter`);
        }
      }
    }
  }

  for (const [key, filter] of Object.entries(filters)) {
    filter.setReRender((newF) => onFilterUpdate({ ...filters, [key]: newF }));
  }

  return {
    parseError:
      parseErrors.length === 0
        ? undefined
        : parseErrors.filter((pe) => pe).join(', '),
    filters: filters as FilterValuesFromBuilders<T>,
  };
}

function combineWrappedResults(wrapped: ParseResult[]) {
  const out: ParseResult = { isError: false, terms: {} };

  for (const w of wrapped) {
    if (w.isError) return w;

    for (const [key, val] of Object.entries(w.terms)) {
      if (!out.terms[key]) {
        out.terms[key] = [];
      }
      out.terms[key].push(...val);
    }
  }
  return out;
}
