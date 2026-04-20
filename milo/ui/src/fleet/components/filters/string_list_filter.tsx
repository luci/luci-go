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

import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Button, Box, Divider, MenuList } from '@mui/material';
import _ from 'lodash';
import {
  useRef,
  useState,
  useImperativeHandle,
  useDeferredValue,
  useMemo,
} from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { colors } from '@/fleet/theme/colors';
import { OptionValue } from '@/fleet/types/option';
import * as ast from '@/fleet/utils/aip160/ast/ast';
import { fuzzySort, fuzzyMaxScore } from '@/fleet/utils/fuzzy_sort';

import { OptionsMenu } from '../filter_dropdown/options_menu';
import { Footer } from '../options_dropdown/footer';
import { SegmentedToggle } from '../segmented_toggle/segmented_toggle';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import {
  BuildResult,
  FilterCategory,
  FilterCategoryBuilder,
  memberToKey,
} from './use_filters';

const ANY_VALUE = '*';

interface OptionWithSelection {
  optionValue: OptionValue;
  isSelected: boolean;
}

export class StringListFilterCategory implements FilterCategory {
  private options: Record<string, OptionWithSelection>;

  public label: string;
  public key: string;
  public isExcluded: boolean = false;
  private reRender: () => void;

  public getOptions() {
    return this.options;
  }

  private constructor(
    label: string,
    key: string,
    options: Record<string, OptionWithSelection>,
    reRender: (newFilter: StringListFilterCategory) => void,
  ) {
    this.label = label;
    this.key = key;
    this.options = options;
    this.reRender = () => {
      reRender(this);
    };
  }

  public static create(
    label: string,
    key: string,
    options: OptionValue[],
    defaultOptions: string[],
    reRender: (newFilter: StringListFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ): BuildResult<StringListFilterCategory> {
    const warnings: string[] = [];
    const optionsMap: Record<string, OptionWithSelection> = Object.fromEntries(
      options.map((o) => [
        o.value,
        {
          optionValue: o,
          isSelected: terms === null ? defaultOptions.includes(o.value) : false,
        },
      ]),
    );

    const category = new StringListFilterCategory(
      label,
      key,
      optionsMap,
      reRender,
    );

    if (terms === null) {
      return { isError: false, value: category, warnings };
    }
    let hasPositive = false;
    let hasNegative = false;

    const processTermValue = (value: string, isNegated: boolean) => {
      if (!optionsMap[value] && value !== ANY_VALUE) {
        return `Option ${value} doesn't exist for ${key}`;
      }

      const isBlank = value === ANY_VALUE;

      // NOT label:* means Is Blank (Include)
      const selectsPositive = isBlank ? isNegated : !isNegated;
      // label:* means Not Blank (Exclude)
      const selectsNegative = isBlank ? !isNegated : isNegated;

      if (selectsPositive) hasPositive = true;
      if (selectsNegative) hasNegative = true;

      const targetValue = isBlank ? BLANK_VALUE : value;
      optionsMap[targetValue].isSelected = true;
      return undefined;
    };

    for (const term of terms) {
      if (
        term.simple.comparator !== ':' &&
        term.simple.comparator !== '=' &&
        term.simple.comparator !== '!='
      ) {
        return {
          isError: true,
          error: 'StringListFilterCategory only supports :, = or != comparator',
        };
      }

      const isNegated = term.negated || term.simple.comparator === '!=';
      const arg = term.simple.arg;
      if (!arg) {
        return {
          isError: true,
          error: `${key} restriction must have an argument.`,
        };
      }
      switch (arg.kind) {
        case 'Comparable':
          const err = processTermValue(memberToKey(arg.member), isNegated);
          if (err) {
            return { isError: true, error: err };
          }
          break;
        case 'Expression':
          if (arg.sequences.length !== 1) {
            return {
              isError: true,
              error: `${key} = (... AND ...) is not supported.`,
            };
          }

          const sequence = arg.sequences[0];
          if (sequence.factors.length !== 1) {
            return {
              isError: true,
              error: `${key} = (... AND ...) is not supported.`,
            };
          }
          const factor = sequence.factors[0];
          for (const innerTerm of factor.terms) {
            if (innerTerm.simple.kind !== 'Restriction') {
              return {
                isError: true,
                error: `${key} = (value OR (...)) is not supported`,
              };
            }
            if (innerTerm.simple.arg !== null) {
              return {
                isError: true,
                error: `${key} = (... key = value ...) is not supported`,
              };
            }

            const innerValue = memberToKey(innerTerm.simple.comparable.member);
            const err = processTermValue(
              innerValue,
              isNegated ? !innerTerm.negated : innerTerm.negated,
            );
            if (err) {
              return { isError: true, error: err };
            }
          }
          break;
      }
    }

    if (hasPositive && hasNegative) {
      return {
        isError: true,
        error: `Mixed inclusion and exclusion is not supported for ${key}`,
      };
    }

    category.isExcluded = hasNegative;
    return { isError: false, value: category, warnings };
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

    const isBlankSelected =
      this.options[BLANK_VALUE] && this.options[BLANK_VALUE].isSelected;

    const blankPart = isBlankSelected
      ? this.isExcluded
        ? `${this.key}:*`
        : `NOT ${this.key}:*`
      : '';

    const regularPart =
      selectedValues.length > 0
        ? this.isExcluded
          ? selectedValues.map((v) => `${this.key} != ${v}`).join(' AND ')
          : selectedValues.map((v) => `${this.key} = ${v}`).join(' OR ')
        : '';

    if (!regularPart) return blankPart;
    if (!blankPart) return `(${regularPart})`;

    return this.isExcluded
      ? `(${regularPart} AND ${blankPart})`
      : `(${blankPart} OR ${regularPart})`;
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
          Object.fromEntries(
            Object.values(this.options).map((o) => [
              o.optionValue.value,
              o.isSelected,
            ]),
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
        isExcluded={this.isExcluded}
        childrenSearchQuery={childrenSearchQuery}
        onNavigateUp={onNavigateUp}
        options={this.options}
        onApply={(newOpt, newIsExcluded) => {
          this.options = newOpt;
          this.isExcluded = newIsExcluded;
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

    const operator = this.isExcluded ? 'NOT IN' : 'IN';
    return `${selectedLabels.length} | ${this.label} ${operator} ${selectedLabels.join(
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

  public setSelectedOptions(selectedKeys: string[], silent = false) {
    const unquotedSelectedKeys = new Set(
      selectedKeys.map((k) => k.replace(/^"(.*)"$/, '$1')),
    );

    const map: Record<string, boolean> = {};
    const foundKeys = new Set<string>();

    for (const opt of Object.values(this.options)) {
      const unquotedOptKey = opt.optionValue.value.replace(/^"(.*)"$/, '$1');
      const isSelected = unquotedSelectedKeys.has(unquotedOptKey);

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
  isExcluded: initialIsExcluded,
  ref,
}: {
  filterKey: string;
  childrenSearchQuery: string;
  onNavigateUp: (e: React.KeyboardEvent) => void;
  options: Record<string, OptionWithSelection>;
  onApply: (
    opt: Record<string, OptionWithSelection>,
    isExcluded: boolean,
  ) => void;
  onClose: () => void;
  isExcluded: boolean;
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
  const [isExcluded, setIsExcluded] = useState(initialIsExcluded);
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
        const isASelected = a.el.isSelected;
        const isBSelected = b.el.isSelected;

        if (isASelected && !isBSelected && a.score >= 0) return -1;
        if (isBSelected && !isASelected && b.score >= 0) return 1;

        if (a.score !== b.score) {
          return b.score - a.score;
        }

        const inScopeA = a.el.optionValue.inScope ?? true;
        const inScopeB = b.el.optionValue.inScope ?? true;

        if (inScopeA !== inScopeB) {
          return inScopeA ? -1 : 1;
        }

        return 0;
      })
      .map((a) => ({
        ...a,
        el: {
          ...a.el.optionValue,
          isSignificant: isNaN(threshold) || a.score >= threshold,
        } as OptionValue,
      }));
  }, [deferredSearchQuery, options]);

  const selectableOptions = useMemo(
    () =>
      fuzzySorted.filter(
        (option) =>
          option.el.isSignificant !== false && option.el.inScope !== false,
      ),
    [fuzzySorted],
  );

  const areAllSelectableOptionsSelected =
    selectableOptions.length > 0 &&
    selectableOptions.every(
      (option) => tempOptions[option.el.value]?.isSelected,
    );

  const handleSelectClearAll = () => {
    setTempOptions((previousOptions) => {
      const updatedOptions = { ...previousOptions };
      const allSelectableAreSelected =
        selectableOptions.length > 0 &&
        selectableOptions.every(
          (option) => previousOptions[option.el.value]?.isSelected,
        );

      if (allSelectableAreSelected) {
        for (const option of fuzzySorted) {
          updatedOptions[option.el.value] = {
            ...updatedOptions[option.el.value],
            isSelected: false,
          };
        }
      } else {
        for (const option of selectableOptions) {
          updatedOptions[option.el.value] = {
            ...updatedOptions[option.el.value],
            isSelected: true,
          };
        }
      }
      return updatedOptions;
    });
  };

  const selectedCount = Object.values(tempOptions).filter(
    (obj) => obj.isSelected,
  ).length;
  const applyDisabled = selectedCount > 200;

  const handleApply = () => {
    if (applyDisabled) {
      return;
    }
    onApply(tempOptions, isExcluded);
  };

  const ExclusionBoxIcon = (
    <div
      style={{
        width: 20,
        height: 20,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <CloseIcon
        sx={{
          fontSize: 12,
          width: 16,
          height: 16,
          backgroundColor: colors.red[500],
          borderRadius: '2px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#ffffff',
        }}
      />
    </div>
  );

  return (
    <div
      role="presentation"
      key={filterKey}
      onKeyDown={(e) => {
        filterDropdownKeyDown(e, handleApply, onClose);
      }}
    >
      <Box sx={{ padding: '0 4px', display: 'flex', justifyContent: 'center' }}>
        <SegmentedToggle
          options={[
            {
              value: 'Include',
              label: 'Include',
              icon: <CheckIcon fontSize="small" />,
            },
            {
              value: 'Exclude',
              label: 'Exclude',
              icon: <CloseIcon fontSize="small" />,
            },
          ]}
          value={isExcluded ? 'Exclude' : 'Include'}
          onChange={(newValue) => setIsExcluded(newValue === 'Exclude')}
        />
      </Box>
      <div>
        <div
          css={{
            display: 'flex',
            gap: 12,
            padding: '0px 4px',
            alignItems: 'center',
            justifyContent: 'space-between',
            flexDirection: 'row-reverse',
          }}
        >
          <Button disableElevation onClick={handleSelectClearAll} tabIndex={-1}>
            {areAllSelectableOptionsSelected ? 'Clear All' : 'Select All'}
          </Button>
        </div>
        <Divider
          sx={{
            backgroundColor: 'transparent',
            paddingTop: 0,
            marginTop: 0,
            height: 1,
          }}
        />
      </div>
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
          checkedIcon={isExcluded ? ExclusionBoxIcon : undefined}
        />
      </MenuList>
      <Footer
        onCancelClick={onClose}
        onApplyClick={handleApply}
        applyDisabled={applyDisabled}
        applyTooltip={
          applyDisabled ? 'Cannot select more than 200 options.' : undefined
        }
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
  ): BuildResult<StringListFilterCategory> {
    if (!this.isFilledIn()) {
      return {
        isError: true,
        error: 'StringListFilterCategoryBuilder is not filled in',
      };
    }

    return StringListFilterCategory.create(
      this.label!,
      key,
      this.options!,
      this.defaultOptions,
      reRender,
      terms,
    );
  }
}
