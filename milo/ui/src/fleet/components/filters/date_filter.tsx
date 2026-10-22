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

import { DateTime } from 'luxon';
import { useImperativeHandle, useMemo, useRef, useState } from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { DateFilterValue } from '@/fleet/types';
import * as ast from '@/fleet/utils/aip160/ast/ast';

import { DateFilter } from '../filter_dropdown/date_filter';
import { OptionComponentHandle } from '../filter_dropdown/filter_dropdown';
import { Footer } from '../options_dropdown/footer';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import { unquote } from './normalize_filter_key';
import {
  BuildResult,
  FilterCategory,
  FilterCategoryBuilder,
} from './use_filters';

export class DateFilterCategoryData implements FilterCategory {
  public value: DateFilterValue;

  public label: string;
  public key: string;

  // if true the date sent to the backend will be in format YYYY-MM-DD which is useful e.g. in RRI
  // where we don't know the time and timezones.
  // in the future this may be expanded into 2 separate filters - one with time and the other date only
  public isDateOnly: boolean;
  public isFutureDisabled?: boolean;
  private reRender: () => void;

  private constructor(
    label: string,
    key: string,
    value: DateFilterValue,
    reRender: (newFilter: DateFilterCategoryData) => void,
    isDateOnly = false,
    isFutureDisabled?: boolean,
  ) {
    this.label = label;
    this.key = key;
    this.value = value;
    this.isDateOnly = isDateOnly;
    this.reRender = () => {
      reRender(this);
    };
    this.isFutureDisabled = isFutureDisabled;
  }

  public static create(
    label: string,
    key: string,
    reRender: (newFilter: DateFilterCategoryData) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
    isDateOnly = false,
    isFutureDisabled?: boolean,
  ): BuildResult<DateFilterCategoryData> {
    const value: DateFilterValue = {};
    const warnings: string[] = [];

    if (terms !== null) {
      for (const term of terms) {
        if (
          term.simple.comparator === ':' &&
          term.simple.arg?.kind === 'Comparable' &&
          term.simple.arg.member.value.value === '*'
        ) {
          if (term.negated) {
            value.onlyEmpty = true;
            value.excludeEmpty = undefined;
          } else {
            value.excludeEmpty = true;
            value.onlyEmpty = undefined;
          }
          continue;
        }
        if (term.negated) {
          continue;
        }

        if (term.simple.arg?.kind !== 'Comparable') {
          continue;
        }

        const valStr = term.simple.arg.member.value.value;
        let dt: DateTime;
        try {
          dt = DateTime.fromISO(valStr, { zone: 'utc' });
          if (!dt.isValid) {
            warnings.push(`Invalid date "${valStr}" for ${key}`);
            continue;
          }
        } catch (_e) {
          warnings.push(`Invalid date "${valStr}" for ${key}`);
          continue;
        }
        const date = dt.toJSDate();

        const comparator = term.simple.comparator;
        if (comparator === '>=' || comparator === '>') {
          value.min = date;
        } else if (comparator === '<=' || comparator === '<') {
          value.max = date;
        } else if (comparator === '=' || comparator === ':') {
          value.min = date;
          value.max = date;
        }
      }
    }

    const filter = new DateFilterCategoryData(
      label,
      key,
      value,
      reRender,
      isDateOnly,
      isFutureDisabled,
    );
    return { isError: false, value: filter, warnings };
  }

  public setReRender(reRender: (newFilter: FilterCategory) => void) {
    this.reRender = () => {
      reRender(this);
    };
  }

  public toAIP160(): string {
    const safeKey = this.key.trim();
    const rangeParts: string[] = [];
    if (this.value.min) {
      const val = this.isDateOnly
        ? DateTime.fromJSDate(this.value.min).toISODate()
        : this.value.min.toISOString();
      rangeParts.push(`${safeKey} >= "${val}"`);
    }
    if (this.value.max) {
      const val = this.isDateOnly
        ? DateTime.fromJSDate(this.value.max).toISODate()
        : this.value.max.toISOString();
      rangeParts.push(`${safeKey} <= "${val}"`);
    }
    const rangeExpr = rangeParts.join(' AND ');

    if (this.value.onlyEmpty) {
      if (!rangeExpr) {
        return `NOT ${safeKey}:*`;
      }
      return `(NOT ${safeKey}:* OR (${rangeExpr}))`;
    }
    if (this.value.excludeEmpty) {
      if (!rangeExpr) {
        return `${safeKey}:*`;
      }
      return `${safeKey}:* AND ${rangeExpr}`;
    }
    return rangeExpr;
  }

  public render(
    _childrenSearchQuery: string,
    _onNavigateUp: (e: React.KeyboardEvent) => void,
    onApply: () => void,
    onClose: () => void,
    ref?: React.Ref<OptionComponentHandle>,
  ) {
    return (
      <DateOptionComponent
        key={'date_filter' + this.key}
        value={this.value}
        onApply={(newValue) => {
          this.value = newValue;
          onApply();
          this.reRender();
        }}
        onClose={onClose}
        ref={ref}
        isFutureDisabled={this.isFutureDisabled}
        isDateOnly={this.isDateOnly}
      />
    );
  }

  public getChipLabel() {
    const formatOpts = this.isDateOnly
      ? DateTime.DATE_SHORT
      : DateTime.DATETIME_SHORT;
    const rangeParts: string[] = [];
    if (this.value.min) {
      rangeParts.push(
        `from ${DateTime.fromJSDate(this.value.min).toLocaleString(formatOpts)}`,
      );
    }
    if (this.value.max) {
      rangeParts.push(
        `to ${DateTime.fromJSDate(this.value.max).toLocaleString(formatOpts)}`,
      );
    }
    if (this.value.onlyEmpty) {
      if (rangeParts.length === 0) {
        return `1 | ${this.label} IN ${BLANK_VALUE}`;
      }
      return `2 | ${this.label} ${rangeParts.join(' ')}, ${BLANK_VALUE}`;
    }
    if (this.value.excludeEmpty) {
      if (rangeParts.length === 0) {
        return `1 | ${this.label} NOT IN ${BLANK_VALUE}`;
      }
      return `2 | ${this.label} ${rangeParts.join(' ')} (NOT ${BLANK_VALUE})`;
    }
    return `1 | ${this.label} ${rangeParts.join(' ')}`;
  }

  public isActive() {
    return !!(
      this.value.min ||
      this.value.max ||
      this.value.onlyEmpty ||
      this.value.excludeEmpty
    );
  }

  public clear() {
    this.value = {};
    this.reRender();
  }

  public setSelectedOptions(
    selectedKeys: string[],
    silent = false,
  ): string | undefined {
    const hasBlank = selectedKeys.some(
      (k) => unquote(k).toLowerCase() === BLANK_VALUE.toLowerCase(),
    );
    this.value = hasBlank ? { onlyEmpty: true } : {};
    if (!silent) {
      this.reRender();
    }
    return undefined;
  }

  public getChildrenSearchScore(_searchQuery: string) {
    return 0;
  }
}

const DateOptionComponent = function DateOptionComponent({
  value,
  onApply,
  onClose,
  ref,
  isFutureDisabled,
  isDateOnly,
}: {
  value: DateFilterValue;
  onApply: (value: DateFilterValue) => void;
  onClose: () => void;
  ref?: React.Ref<unknown>;
  isFutureDisabled?: boolean;
  isDateOnly?: boolean;
}) {
  const [tempValue, setTempValue] = useState(value);
  const innerRef = useRef<OptionComponentHandle>(null);
  useImperativeHandle(ref, () => ({
    focus: () => {
      innerRef.current?.focus();
    },
  }));
  const isInvalidRange = useMemo(
    () => tempValue.min && tempValue.max && tempValue.max < tempValue.min,
    [tempValue],
  );

  return (
    <div
      role="presentation"
      css={{
        '& .options-menu-footer': {
          padding: '10px 16px 12px 16px',
        },
      }}
      onKeyDown={(e) => {
        filterDropdownKeyDown(
          e,
          () => !isInvalidRange && onApply(tempValue),
          onClose,
        );
      }}
    >
      <DateFilter
        ref={innerRef}
        value={tempValue}
        onChange={setTempValue}
        isFutureDisabled={isFutureDisabled}
        isDateOnly={isDateOnly}
      />
      <Footer
        applyDisabled={
          tempValue.min && tempValue.max && tempValue.max < tempValue.min
        }
        applyTooltip="The value of `From` cannot be more recent than the value of `To`"
        onCancelClick={onClose}
        onApplyClick={() => {
          onApply(tempValue);
        }}
      />
    </div>
  );
};

export class DateFilterCategoryDataBuilder
  implements FilterCategoryBuilder<DateFilterCategoryData>
{
  public label: string | undefined;
  public isDateOnly = false;
  public isFutureDisabled = false;

  constructor() {}

  public setLabel(label: string) {
    this.label = label;
    return this;
  }

  public disableFuture() {
    this.isFutureDisabled = true;
    return this;
  }

  public setIsDateOnly(isDateOnly: boolean) {
    this.isDateOnly = isDateOnly;
    return this;
  }

  public isFilledIn(): boolean {
    return this.label !== undefined;
  }

  public build(
    key: string,
    reRender: (newFilter: DateFilterCategoryData) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ): BuildResult<DateFilterCategoryData> {
    const { label, isDateOnly, isFutureDisabled } = this;
    if (label === undefined) {
      return {
        isError: true,
        error: 'DateFilterCategoryDataBuilder is not filled in',
      };
    }

    return DateFilterCategoryData.create(
      label,
      key,
      reRender,
      terms,
      isDateOnly,
      isFutureDisabled,
    );
  }
}
