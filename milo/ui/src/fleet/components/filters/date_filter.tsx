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
import { useImperativeHandle, useRef, useState } from 'react';

import { DateFilterValue } from '@/fleet/types';
import * as ast from '@/fleet/utils/aip160/ast/ast';

import { DateFilter } from '../filter_dropdown/date_filter';
import { OptionComponentHandle } from '../filter_dropdown/filter_dropdown';
import { Footer } from '../options_dropdown/footer';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import { FilterCategory, FilterCategoryBuilder } from './use_filters';

export class DateFilterCategoryData implements FilterCategory {
  public value: DateFilterValue;

  public label: string;
  public key: string;
  private reRender: () => void;
  private actualReRender: (newFilter: DateFilterCategoryData) => void;

  constructor(
    label: string,
    key: string,
    reRender: (newFilter: DateFilterCategoryData) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] = [],
  ) {
    this.label = label;
    this.key = key;
    this.value = {};
    this.actualReRender = reRender;
    this.reRender = () => {
      reRender(this);
    };

    for (const term of terms) {
      if (term.negated) {
        continue;
      }

      if (term.simple.arg?.kind !== 'Comparable') {
        continue;
      }

      const valStr = term.simple.arg.member.value.value;
      const date = DateTime.fromISO(valStr, { zone: 'utc' }).toJSDate();

      if (isNaN(date.getTime())) {
        continue;
      }

      const comparator = term.simple.comparator;
      if (comparator === '>=' || comparator === '>') {
        this.value.min = date;
      } else if (comparator === '<=' || comparator === '<') {
        this.value.max = date;
      } else if (comparator === '=' || comparator === ':') {
        this.value.min = date;
        this.value.max = date;
      }
    }
  }

  public clone() {
    const newInst = new DateFilterCategoryData(
      this.label,
      this.key,
      this.actualReRender,
    );
    newInst.value = { ...this.value };
    return newInst as FilterCategory;
  }

  public toAIP160(): string {
    const parts: string[] = [];
    const safeKey = this.key.trim();
    if (this.value.min) {
      parts.push(`${safeKey} >= "${this.value.min.toISOString()}"`);
    }
    if (this.value.max) {
      parts.push(`${safeKey} <= "${this.value.max.toISOString()}"`);
    }
    return parts.join(' AND ');
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
      />
    );
  }

  public getChipLabel() {
    const parts: string[] = [];
    if (this.value.min) {
      parts.push(
        `from ${DateTime.fromJSDate(this.value.min).toLocaleString()}`,
      );
    }
    if (this.value.max) {
      parts.push(`to ${DateTime.fromJSDate(this.value.max).toLocaleString()}`);
    }
    return `[ ${this.label} ]: ${parts.join(' ')}`;
  }

  public isActive() {
    return !!(this.value.min || this.value.max);
  }

  public clear() {
    this.value = {};
    this.reRender();
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
}: {
  value: DateFilterValue;
  onApply: (value: DateFilterValue) => void;
  onClose: () => void;
  ref?: React.Ref<unknown>;
}) {
  const [tempValue, setTempValue] = useState(value);
  const innerRef = useRef<OptionComponentHandle>(null);
  useImperativeHandle(ref, () => ({
    focus: () => {
      innerRef.current?.focus();
    },
  }));

  return (
    <div
      role="presentation"
      onKeyDown={(e) => {
        filterDropdownKeyDown(e, () => onApply(tempValue), onClose);
      }}
    >
      <DateFilter ref={innerRef} value={tempValue} onChange={setTempValue} />
      <Footer
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

  constructor() {}

  public setLabel(label: string) {
    this.label = label;
    return this;
  }

  public isFilledIn() {
    return this.label !== undefined;
  }

  public build(
    key: string,
    reRender: (newFilter: DateFilterCategoryData) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | undefined,
  ) {
    if (!this.isFilledIn()) {
      throw new Error('DateFilterCategoryDataBuilder is not filled in');
    }

    return new DateFilterCategoryData(this.label!, key, reRender, terms);
  }
}
