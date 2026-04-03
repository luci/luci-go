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
import { useRef, useMemo, useState, useImperativeHandle } from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as ast from '@/fleet/utils/aip160/ast/ast';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { OptionsMenu } from '../filter_dropdown/options_menu';
import { Footer } from '../options_dropdown/footer';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import {
  FilterCategory,
  FilterCategoryBuilder,
  memberToKey,
} from './use_filters';

export class StringListFilterCategory implements FilterCategory {
  private options: Record<
    string,
    { label: string; isSelected: boolean; key: string }
  >;

  public label: string;
  public key: string;
  private reRender: () => void;
  private actualReRender: (newFilter: StringListFilterCategory) => void;

  public getOptions() {
    return this.options;
  }

  constructor(
    label: string,
    key: string,
    options: { label: string; key: string }[],
    reRender: (newFilter: StringListFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] = [],
  ) {
    this.label = label;
    this.key = key;
    this.options = Object.fromEntries(
      options.map((o) => [o.key, { ...o, isSelected: false }]),
    );
    this.actualReRender = reRender;
    this.reRender = () => {
      reRender(this);
    };

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

  public clone() {
    const newInst = new StringListFilterCategory(
      this.label,
      this.key,
      [],
      this.actualReRender,
    );
    newInst.options = this.options;
    return newInst as FilterCategory;
  }

  public toAIP160(): string {
    const selectedValues = Object.values(this.options)
      .filter((o) => o.isSelected)
      .filter((o) => o.key !== BLANK_VALUE)
      .map((o) => {
        const val = o.key;
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
      opt.isSelected = !!newOptions[opt.key];
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
      .map((o) => o.label);

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
      .map((o) => o.key);
  }

  public setSelectedOptions(selectedKeys: string[], silent = false) {
    const unquotedSelectedKeys = selectedKeys.map((k) =>
      k.replace(/^"(.*)"$/, '$1'),
    );
    const map: Record<string, boolean> = {};
    for (const opt of Object.values(this.options)) {
      const unquotedOptKey = opt.key.replace(/^"(.*)"$/, '$1');
      map[opt.key] = unquotedSelectedKeys.includes(unquotedOptKey);
    }
    this.setOptions(map, silent);
  }

  public getChildrenSearchScore(searchQuery: string) {
    const sortedChildren = fuzzySort(searchQuery)(
      Object.values(this.options),
      (o) => o.label,
    );
    return sortedChildren[0]?.score;
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
  options: Record<string, { label: string; isSelected: boolean; key: string }>;
  onApply: (
    opt: Record<string, { label: string; isSelected: boolean; key: string }>,
  ) => void;
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
  const fuzzySorted = useMemo(
    () =>
      fuzzySort(childrenSearchQuery)(
        Object.values(options),
        (o) => o.label,
      ).sort((a, b) => {
        const isASelected = options[a.el.key].isSelected;
        const isBSelected = options[b.el.key].isSelected;

        if (isASelected && !isBSelected && a.score >= 0) return -1;
        if (isBSelected && !isASelected && b.score >= 0) return 1;

        return b.score - a.score;
      }),
    [childrenSearchQuery, options],
  );

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
          elements={fuzzySorted.map((e) => ({
            ...e,
            el: { label: e.el.label, value: e.el.key },
          }))}
          selectedElements={
            new Set(
              Object.values(tempOptions)
                .filter((o) => o.isSelected)
                .map((o) => o.key),
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
  public options: { label: string; key: string }[] | undefined;

  constructor() {}

  public setLabel(label: string) {
    this.label = label;
    return this;
  }
  public setOptions(options: { label: string; key: string }[]) {
    this.options = options;
    return this;
  }

  public isFilledIn() {
    return this.label !== undefined && this.options !== undefined;
  }

  public build(
    key: string,
    reRender: (newFilter: StringListFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | undefined,
  ) {
    if (!this.isFilledIn())
      throw new Error(
        'StringListFilterCategoryBuilder is not filled in :' + this,
      );

    return new StringListFilterCategory(
      this.label!,
      key!,
      this.options!,
      reRender,
      terms,
    );
  }
}
