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

import _ from 'lodash';
import { useCallback, useEffect, useMemo, useState } from 'react';

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
): {
  filterValues: FilterValuesFromBuilders<T> | undefined;
  aip160: string;
  parseError: string | undefined;
} => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [filtersAIP160, setFiltersAIP160] = useState(
    searchParams.get(FILTERS_PARAM_KEY) ?? '',
  );

  useEffect(() => {
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      newParams.set(FILTERS_PARAM_KEY, filtersAIP160);
      return newParams;
    });
  }, [filtersAIP160, setSearchParams]);

  // Stabilize builders to prevent infinite loops if the parent passes a new object every render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const builders = useMemo(() => rawBuilders, [JSON.stringify(rawBuilders)]);

  const onFilterUpdate = useCallback(
    function onFilterUpdate(key: string, newFilterValue: FilterCategory) {
      setFilterValues((newF) => {
        const newFilterCategories = {
          ...newF,
          [key]: newFilterValue.clone(),
        } as FilterValuesFromBuilders<T>;

        const currentAIP160 = Object.values(newFilterCategories)
          .filter((newFilterCategories) => newFilterCategories.isActive())
          .map((f) => f.toAIP160())
          .filter((f) => f !== '')
          .join(' AND ');

        if (currentAIP160 !== filtersAIP160) {
          setFiltersAIP160(currentAIP160);
          setParseError(undefined);
        }

        return newFilterCategories;
      });
    },
    [filtersAIP160, setFiltersAIP160],
  );
  const [parseError, setParseError] = useState<string | undefined>();
  const [filterValues, setFilterValues] = useState<
    FilterValuesFromBuilders<T> | undefined
  >(() => {
    const { filters, parseError } = buildFilters(
      builders!, //TODO: This is a lie
      onFilterUpdate,
      filtersAIP160,
    );

    setParseError(parseError);
    return filters;
  });

  // When the builders change, create new filterCategories taking the values from the url
  useEffect(() => {
    if (builders === undefined) return;

    setParseError(undefined);
    const { filters, parseError } = buildFilters(
      builders,
      onFilterUpdate,
      filtersAIP160,
    );

    setParseError(parseError);
    setFilterValues(filters);
  }, [builders, filtersAIP160, onFilterUpdate]);

  return useMemo(
    () => ({
      filterValues: filterValues,
      aip160: filtersAIP160,
      parseError,
    }),
    [filterValues, filtersAIP160, parseError],
  );
};

export interface FilterCategoryBuilder<T extends FilterCategory> {
  isFilledIn(): boolean;
  build(key: string, reRender: (newFilter: T) => void): T;
}

export interface FilterCategory {
  key: string;
  label: string;

  fromAIP160(s: (ast.Term & { simple: ast.Restriction })[]): void;
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

  clone(): FilterCategory;
}

type ParseResult =
  | {
      isError: false;
      terms: Record<string, (ast.Term & { simple: ast.Restriction })[]>;
    }
  | { isError: true; error: string };

const constructFiltersFromAIP160 = (filtersAIP160: string) => {
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

  return wrapper(parseFilter(filtersAIP160));
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
  onFilterUpdate: (key: string, newFilterValue: FilterCategory) => void,
  filtersAIP160: string,
): {
  filters: FilterValuesFromBuilders<T> | undefined;
  parseError: string | undefined;
} {
  const parseResult = constructFiltersFromAIP160(filtersAIP160);
  if (parseResult.isError) {
    return { filters: undefined, parseError: parseResult.error };
  }

  const filters = Object.fromEntries(
    Object.entries(builders)
      .map(([key, bob]) => {
        if (!bob.isFilledIn()) {
          throw new Error(`Builder ${key} is not filled in: ${bob}`);
        }
        const update = (newFilterValue: FilterCategory) => {
          onFilterUpdate(key, newFilterValue);
        };

        return bob.build(key, update);
      })
      .map((f) => [f.key, f]),
  );

  const parseErrors = Object.entries(parseResult.terms).map(([key, terms]) => {
    if (!filters[key]) {
      filters[key] = new LoadingFilterCategory(key);
      filters[key].fromAIP160(terms);
    }
    try {
      filters[key].fromAIP160(terms);
    } catch (e) {
      if (e instanceof Error) return e.message;

      return String(e);
    }

    return '';
  });

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
