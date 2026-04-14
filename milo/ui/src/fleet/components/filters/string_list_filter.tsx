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

import { MenuList } from '@mui/material';
import _ from 'lodash';
import {
  useRef,
  useState,
  useImperativeHandle,
  useDeferredValue,
  useMemo,
} from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionValue } from '@/fleet/types/option';
import * as ast from '@/fleet/utils/aip160/ast/ast';
import { fuzzySort, fuzzyMaxScore } from '@/fleet/utils/fuzzy_sort';

import { OptionsMenu } from '../filter_dropdown/options_menu';
import { Footer } from '../options_dropdown/footer';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import {
  FilterCategory,
  FilterCategoryBuilder,
  memberToKey,
} from './use_filters';

interface OptionWithSelection {
  optionValue: OptionValue;
  isSelected: boolean;
}

export class StringListFilterCategory implements FilterCategory {
  private options: Record<string, OptionWithSelection>;

  public label: string;
  public key: string;
  private reRender: () => void;

  public getOptions() {
    return this.options;
  }

  constructor(
    label: string,
    key: string,
    options: OptionValue[],
    defaultOptions: string[],
    reRender: (newFilter: StringListFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ) {
    this.label = label;
    this.key = key;
    this.options = Object.fromEntries(
      options.map((o) => [
        o.value,
        {
          optionValue: o,
          isSelected: terms === null ? defaultOptions.includes(o.value) : false,
        },
      ]),
    );
    this.reRender = () => {
      reRender(this);
    };

    if (terms === null) return;
    for (const term of terms) {
      if (!term.simple.arg) {
        if (!term.negated) {
          throw new Error(
            `Found ${this.key}. Expected "NOT ${this.key}" or "${this.key} = ..."`,
          );
        }
        if (!this.options[BLANK_VALUE]) {
          throw new Error(
            `Option ${BLANK_VALUE} doesn't exist for ${this.key}`,
          );
        }

        this.options[BLANK_VALUE].isSelected = true;
        continue;
      }

      if (term.simple.comparator !== ':' && term.simple.comparator !== '=') {
        throw new Error(
          'StringListFilterCategory only supports : or = comparator',
        );
      }

      const arg = term.simple.arg;
      switch (arg.kind) {
        case 'Comparable':
          const value = memberToKey(arg.member);
          if (!this.options[value]) {
            throw new Error(`Option ${value} doesn't exist for ${this.key}`);
          }
          this.options[value].isSelected = true;
          break;
        case 'Expression':
          if (arg.sequences.length !== 1) {
            throw new Error(`${this.key} = (... AND ...) is not supported.`);
          }

          const sequence = arg.sequences[0];
          if (sequence.factors.length !== 1) {
            throw new Error(`${this.key} = (... AND ...) is not supported.`);
          }
          const factor = sequence.factors[0];
          for (const term of factor.terms) {
            if (term.simple.kind !== 'Restriction') {
              throw new Error(
                `${this.key} = (value OR (...)) is not supported`,
              );
            }
            if (term.simple.arg !== null) {
              throw new Error(
                `${this.key} = (... key = value ...) is not supported`,
              );
            }

            const value = memberToKey(term.simple.comparable.member);
            if (!this.options[value]) {
              throw new Error(`Option ${value} doesn't exist for ${this.key}`);
            }
            this.options[value].isSelected = true;
          }
          break;
      }
    }
  }

  public setReRender(reRender: (newFilter: FilterCategory) => void) {
    this.reRender = () => {
      reRender(this);
    };
  }

  public toAIP160(): string {
    const selectedValues = Object.values(this.options)
      .filter((o) => o.isSelected)
      .filter((o) => o.optionValue.value !== BLANK_VALUE)
      .map((o) => {
        const val = o.optionValue.value;
        const isQuoted = val.startsWith('"') && val.endsWith('"');
        if (
          !isQuoted &&
          (val.includes(' ') ||
            val.includes('"') ||
            val.includes('(') ||
            val.includes(')'))
        ) {
          // If the value contains special characters, we quote it.
          // Inside a quoted string in AIP-160, only quotes need to be escaped.
          return `"${val.replace(/"/g, '\\"')}"`;
        }
        return val;
      });

    const regularFilters =
      selectedValues.length === 0
        ? ''
        : this.key + ' = (' + selectedValues.join(' OR ') + ')';

    if (this.options[BLANK_VALUE] && this.options[BLANK_VALUE].isSelected) {
      if (regularFilters) return `(NOT ${this.key} OR ${regularFilters})`;

      return `NOT ${this.key}`;
    }

    // IE: (NOT state OR state = ('offline', 'online'))
    return regularFilters;
  }

  public setOptions(
    newOptions:
      | Record<string, boolean>
      | ((old: Record<string, boolean>) => Record<string, boolean>),
    silent = false,
  ): void {
    if (typeof newOptions === 'function') {
      return this.setOptions(
        newOptions(
          Object.keys(this.options).reduce(
            (acc, key) => ({
              ...acc,
              [key]: this.options[key].isSelected,
            }),
            {} as Record<string, boolean>,
          ),
        ),
        silent,
      );
    }

    for (const opt of Object.values(this.options)) {
      opt.isSelected = !!newOptions[opt.optionValue.value];
    }

    if (!silent) {
      this.reRender();
    }
  }

  public render(
    childrenSearchQuery: string,
    onNavigateUp: (e: React.KeyboardEvent) => void,
    onApply: () => void,
    onClose: () => void,
    ref?: React.Ref<unknown>,
  ) {
    return (
      <OptionComponent
        key={'string_list_filter' + this.key}
        filterKey={this.key}
        childrenSearchQuery={childrenSearchQuery}
        onNavigateUp={onNavigateUp}
        options={this.options}
        onApply={(newOpt) => {
          this.options = newOpt;
          this.reRender();
          onApply();
        }}
        onClose={onClose}
        ref={ref}
      />
    );
  }
  public getChipLabel() {
    const selectedLabels = Object.values(this.options)
      .filter((o) => o.isSelected)
      .map((o) => o.optionValue.label);

    return `${selectedLabels.length} | [ ${this.label} ]: ${selectedLabels.join(
      ', ',
    )}`;
  }

  public isActive() {
    return Object.values(this.options).some((o) => o.isSelected);
  }
  public clear() {
    for (const key of Object.keys(this.options)) {
      this.options[key].isSelected = false;
    }
    this.reRender();
  }

  public getSelectedOptions() {
    return Object.values(this.options)
      .filter((o) => o.isSelected)
      .map((o) => o.optionValue.value);
  }

  public setSelectedOptions(
    selectedKeys: string[],
    silent = false,
  ): string | undefined {
    const unquotedSelectedKeys = selectedKeys.map((k) =>
      k.replace(/^"(.*)"$/, '$1'),
    );
    const map: Record<string, boolean> = {};
    const foundKeys = new Set<string>();

    for (const opt of Object.values(this.options)) {
      const unquotedOptKey = opt.optionValue.value.replace(/^"(.*)"$/, '$1');
      const isSelected = unquotedSelectedKeys.includes(unquotedOptKey);
      map[opt.optionValue.value] = isSelected;
      if (isSelected) {
        foundKeys.add(unquotedOptKey);
      }
    }

    this.setOptions(map, silent);

    for (const key of unquotedSelectedKeys) {
      if (!foundKeys.has(key)) {
        return `Invalid option: ${key}`;
      }
    }

    return undefined;
  }

  public getChildrenSearchScore(searchQuery: string) {
    return fuzzyMaxScore(
      searchQuery,
      (o: OptionWithSelection) => o.optionValue.label,
    )(Object.values(this.options));
  }
}

const OptionComponent = function OptionComponent({
  childrenSearchQuery,
  onNavigateUp,
  options,
  onApply,
  onClose,
  filterKey,
  ref,
}: {
  filterKey: string;
  childrenSearchQuery: string;
  onNavigateUp: (e: React.KeyboardEvent) => void;
  options: Record<string, OptionWithSelection>;
  onApply: (opt: Record<string, OptionWithSelection>) => void;

  onClose: () => void;
  ref?: React.Ref<unknown>;
}) {
  const menuListRef = useRef<HTMLUListElement>(null);
  useImperativeHandle(ref, () => ({
    focus: () => {
      menuListRef.current
        ?.querySelector<HTMLElement>('[role=menuitem]')
        ?.focus();
    },
  }));

  const [tempOptions, setTempOptions] = useState(options);
  const deferredSearchQuery = useDeferredValue(childrenSearchQuery);

  const fuzzySorted = useMemo(() => {
    const scored = fuzzySort(deferredSearchQuery)(
      Object.values(options),
      (o) => o.optionValue.label,
    );

    const scores = scored.map((s) => s.score).filter((s) => s >= 0);
    const avg = _.sum(scores) / scores.length;
    const sd = Math.sqrt(
      _.sum(scores.map((a) => Math.pow(a - avg, 2))) / scores.length,
    );

    const threshold =
      avg + sd > Math.max(...scores)
        ? Math.max(0, avg - 2 * sd) // some queries are too "left skewed"
        : avg + sd;

    return scored
      .sort((a, b) => {
        const isASelected = options[a.el.optionValue.value].isSelected;
        const isBSelected = options[b.el.optionValue.value].isSelected;

        if (isASelected && !isBSelected && a.score >= 0) return -1;
        if (isBSelected && !isASelected && b.score >= 0) return 1;

        if (a.score !== b.score) {
          return b.score - a.score;
        }

        const inScopeA =
          options[a.el.optionValue.value].optionValue.inScope ?? true;
        const inScopeB =
          options[b.el.optionValue.value].optionValue.inScope ?? true;

        if (inScopeA !== inScopeB) {
          return inScopeA ? -1 : 1;
        }

        return 0;
      })
      .map((a) => {
        return {
          ...a,
          matches: a.matches,
          el: {
            value: a.el.optionValue.value,
            label: a.el.optionValue.label,
            inScope: a.el.optionValue.inScope,
            isSignificant:
              isNaN(threshold) || // happens if no scores are >=0
              a.score >= threshold,
          } as OptionValue,
        };
      });
  }, [deferredSearchQuery, options]);

  return (
    <div
      role="presentation"
      key={filterKey}
      onKeyDown={(e) => {
        filterDropdownKeyDown(e, () => onApply(tempOptions), onClose);
      }}
    >
      <MenuList
        ref={menuListRef}
        variant="selectedMenu"
        sx={{
          maxHeight: 400,
          width: 300,
        }}
      >
        <OptionsMenu
          elements={fuzzySorted}
          selectedElements={
            new Set(
              Object.values(tempOptions)
                .filter((o) => o.isSelected)
                .map((o) => o.optionValue.value),
            )
          }
          flipOption={(key) => {
            setTempOptions((old) => ({
              ...old,
              [key]: {
                ...old[key],
                isSelected: !old[key]?.isSelected,
              },
            }));
          }}
          selectOnly={(key) => {
            setTempOptions((old) => {
              const next = { ...old };
              for (const k of Object.keys(next)) {
                next[k] = { ...next[k], isSelected: k === key };
              }
              return next;
            });
          }}
          onNavigateUp={onNavigateUp}
          onNavigateDown={() => {}} // currently just blocking navigating down from the last element
        />
      </MenuList>
      <Footer
        onCancelClick={onClose}
        onApplyClick={() => {
          onApply(tempOptions);
        }}
      />
    </div>
  );
};

export class StringListFilterCategoryBuilder
  implements FilterCategoryBuilder<StringListFilterCategory>
{
  public label: string | undefined;
  public options: OptionValue[] | undefined;
  public defaultOptions: string[] = [];

  constructor() {}

  public setLabel(label: string) {
    this.label = label;
    return this;
  }
  public setOptions(options: OptionValue[]) {
    this.options = options;
    return this;
  }

  public setDefaultOptions(defaultOptions: string[]) {
    this.defaultOptions = defaultOptions;
    return this;
  }

  public isFilledIn() {
    return this.label !== undefined && this.options !== undefined;
  }

  public build(
    key: string,
    reRender: (newFilter: StringListFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ) {
    if (!this.isFilledIn())
      throw new Error(
        'StringListFilterCategoryBuilder is not filled in :' + this,
      );

    return new StringListFilterCategory(
      this.label!,
      key!,
      this.options!,
      this.defaultOptions,
      reRender,
      terms,
    );
  }
}
