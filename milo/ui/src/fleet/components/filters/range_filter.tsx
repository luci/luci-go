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

import { useImperativeHandle, useRef, useState } from 'react';

import * as ast from '@/fleet/utils/aip160/ast/ast';

import { OptionComponentHandle } from '../filter_dropdown/filter_dropdown';
import { RangeFilter, RangeFilterValue } from '../filter_dropdown/range_filter';
import { Footer } from '../options_dropdown/footer';

import { filterDropdownKeyDown } from './filter_dropdown_keydown';
import {
  BuildResult,
  FilterCategory,
  FilterCategoryBuilder,
} from './use_filters';

export class RangeFilterCategory implements FilterCategory {
  public value: RangeFilterValue;

  public label: string;
  public key: string;
  public min: number;
  public max: number;
  private reRender: () => void;

  private constructor(
    label: string,
    key: string,
    min: number,
    max: number,
    value: RangeFilterValue,
    reRender: (newFilter: RangeFilterCategory) => void,
  ) {
    this.label = label;
    this.key = key;
    this.min = min;
    this.max = max;
    this.value = value;
    this.reRender = () => {
      reRender(this);
    };
  }

  public static create(
    label: string,
    key: string,
    min: number,
    max: number,
    reRender: (newFilter: RangeFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ): BuildResult<RangeFilterCategory> {
    const value: RangeFilterValue = {};

    if (terms !== null) {
      for (const term of terms) {
        if (term.negated) {
          continue;
        }

        if (term.simple.arg?.kind !== 'Comparable') {
          continue;
        }

        const valStr = term.simple.arg.member.value.value;
        const num = parseInt(valStr, 10);
        if (isNaN(num)) {
          continue;
        }

        const comparator = term.simple.comparator;
        if (comparator === '>=') {
          value.min = num;
        } else if (comparator === '<=') {
          value.max = num;
        } else if (comparator === '=') {
          value.min = num;
          value.max = num;
        }
      }
    }

    const filter = new RangeFilterCategory(
      label,
      key,
      min,
      max,
      value,
      reRender,
    );
    return { isError: false, value: filter, warnings: [] };
  }

  public setReRender(reRender: (newFilter: FilterCategory) => void) {
    this.reRender = () => {
      reRender(this);
    };
  }

  public toAIP160(): string {
    const parts: string[] = [];
    const safeKey = this.key.trim();
    if (this.value.min !== undefined) {
      parts.push(`${safeKey} >= ${this.value.min}`);
    }
    if (this.value.max !== undefined) {
      parts.push(`${safeKey} <= ${this.value.max}`);
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
      <RangeOptionComponent
        key={'range_filter' + this.key}
        value={this.value}
        min={this.min}
        max={this.max}
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
    if (this.value.min !== undefined) {
      parts.push(`from ${this.value.min}`);
    }
    if (this.value.max !== undefined) {
      parts.push(`to ${this.value.max}`);
    }
    return `[ ${this.label} ]: ${parts.join(' ')}`;
  }

  public isActive() {
    return this.value.min !== undefined || this.value.max !== undefined;
  }

  public clear() {
    this.value = {};
    this.reRender();
  }

  public getChildrenSearchScore(_searchQuery: string) {
    return 0;
  }
}

const RangeOptionComponent = function RangeOptionComponent({
  value,
  min,
  max,
  onApply,
  onClose,
  ref,
}: {
  value: RangeFilterValue;
  min: number;
  max: number;
  onApply: (value: RangeFilterValue) => void;
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
      <RangeFilter
        ref={innerRef}
        value={tempValue}
        onChange={setTempValue}
        min={min}
        max={max}
      />
      <Footer
        onCancelClick={onClose}
        onApplyClick={() => {
          onApply(tempValue);
        }}
      />
    </div>
  );
};

export class RangeFilterCategoryBuilder
  implements FilterCategoryBuilder<RangeFilterCategory>
{
  public label: string | undefined;
  public min: number | undefined;
  public max: number | undefined;

  constructor() {}

  public setLabel(label: string) {
    this.label = label;
    return this;
  }

  public setMin(min: number) {
    this.min = min;
    return this;
  }

  public setMax(max: number) {
    this.max = max;
    return this;
  }

  public isFilledIn() {
    return (
      this.label !== undefined &&
      this.min !== undefined &&
      this.max !== undefined
    );
  }

  public build(
    key: string,
    reRender: (newFilter: RangeFilterCategory) => void,
    terms: (ast.Term & { simple: ast.Restriction })[] | null,
  ): BuildResult<RangeFilterCategory> {
    if (!this.isFilledIn()) {
      return {
        isError: true,
        error: `RangeFilterCategoryBuilder is not filled in: ${JSON.stringify(this)}`,
      };
    }

    return RangeFilterCategory.create(
      this.label!,
      key,
      this.min!,
      this.max!,
      reRender,
      terms,
    );
  }
}
