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

import { MenuList } from '@mui/material';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';

import {
  DateFilterValue,
  OptionCategory,
  SelectedOptions,
  StringListCategory,
} from '@/fleet/types';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { DateFilter } from '../filter_dropdown/date_filter';
import { FilterBar } from '../filter_dropdown/filter_bar';
import {
  FilterCategoryData,
  OptionComponentHandle,
  OptionComponentProps,
} from '../filter_dropdown/filter_dropdown';
import { OptionsMenu } from '../filter_dropdown/options_menu';

export function DeviceListFilterBar({
  filterOptions,
  selectedOptions,
  onSelectedOptionsChange,
  isLoading,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  isLoading?: boolean;
}) {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState<SelectedOptions>(selectedOptions);

  useEffect(() => {
    setTempSelectedOptions(selectedOptions);
  }, [selectedOptions]);

  const clearSelections = useCallback(() => {
    setTempSelectedOptions(selectedOptions);
  }, [setTempSelectedOptions, selectedOptions]);

  const filterCategoryDatas = filterOptions.map((option) => {
    if (option.type === 'date') {
      const dateFilter: FilterCategoryData<DateOnlyFilterOptionComponentProps> =
        {
          type: 'date',
          label: option.label,
          value: option.value,
          getChildrenSearchScore: () => 0,
          optionsComponent: DateOptionComponent,
          optionsComponentProps: {
            option,
            selectedOptions:
              (tempSelectedOptions[option.value] as DateFilterValue) || {},
            onSelectedOptionsChange: (newSelectedOptions: DateFilterValue) =>
              setTempSelectedOptions({
                ...tempSelectedOptions,
                [option.value]: newSelectedOptions,
              }),
            onClose: clearSelections,
          },
        };
      return dateFilter;
    }

    if (option.type === 'string_list' || option.type === undefined) {
      const stringListOption = option as StringListCategory;
      return {
        type: 'string_list',
        label: option.label,
        value: option.value,
        getChildrenSearchScore: (childrenSearchQuery: string) => {
          const sortedChildren = fuzzySort(childrenSearchQuery)(
            stringListOption.options || [],
            (o) => o.label,
          );
          return sortedChildren[0]?.score;
        },
        optionsComponent: OptionComponent,
        optionsComponentProps: {
          option: option,
          selectedOptions: (tempSelectedOptions[option.value] ?? []) as unknown,
          onSelectedOptionsChange: (newSelectedOptions: string[]) =>
            setTempSelectedOptions({
              ...tempSelectedOptions,
              [option.value]: newSelectedOptions,
            }),
          onClose: clearSelections,
        },
      };
    }

    throw new Error(`Unsupported filter type: ${option.type}`);
  }) as FilterCategoryData<unknown>[];

  const getChipLabel = (filterCategory: FilterCategoryData<unknown>) => {
    if (filterCategory.type === 'date') {
      const dateValue = selectedOptions[
        filterCategory.value
      ] as DateFilterValue;
      if (dateValue?.min && dateValue?.max) {
        return `[ ${filterCategory.label} ]: ${dateValue.min.toISOString().slice(0, 10)} - ${dateValue.max.toISOString().slice(0, 10)}`;
      }
      if (dateValue?.min) {
        return `[ ${filterCategory.label} ]: >= ${dateValue.min.toISOString().slice(0, 10)}`;
      }
      if (dateValue?.max) {
        return `[ ${filterCategory.label} ]: <= ${dateValue.max.toISOString().slice(0, 10)}`;
      }
      return '';
    }

    if (
      filterCategory.type === 'string_list' ||
      filterCategory.type === undefined
    ) {
      const filterOption = filterOptions.find(
        (opt) => opt.value === filterCategory.value,
      );
      if (!filterOption) {
        return '';
      }

      const stringListOption = filterOption as StringListCategory;

      const selectedValues = selectedOptions[filterCategory.value] as
        | string[]
        | undefined;
      if (!selectedValues) return '';

      const optionLabels = selectedValues.map(
        (v) => stringListOption.options?.find((o) => o.value === v)?.label ?? v,
      );

      return `${selectedValues.length ?? 0} | [ ${filterCategory.label} ]: ${optionLabels.join(
        ', ',
      )}`;
    }

    throw new Error(`Unsupported filter type: ${filterCategory.type}`);
  };

  return (
    <FilterBar
      filterCategoryDatas={filterCategoryDatas}
      selectedOptions={Object.keys(selectedOptions)}
      onApply={() => onSelectedOptionsChange(tempSelectedOptions)}
      getChipLabel={(o) => getChipLabel(o)}
      onChipDeleted={(o) => {
        const newOptions = { ...selectedOptions };
        delete newOptions[o.value];
        onSelectedOptionsChange(newOptions);
      }}
      isLoading={isLoading}
    />
  );
}

interface StringOnlyFilterOptionComponentProps {
  option: OptionCategory;
  selectedOptions: string[];
  onSelectedOptionsChange: (newSelectedOptions: string[]) => void;
  onClose: () => void;
}

interface DateOnlyFilterOptionComponentProps {
  option: OptionCategory;
  selectedOptions: DateFilterValue;
  onSelectedOptionsChange: (newSelectedOptions: DateFilterValue) => void;
  onClose: () => void;
}

const DateOptionComponent = forwardRef<
  OptionComponentHandle,
  OptionComponentProps<DateOnlyFilterOptionComponentProps>
>(function DateOptionComponent(
  { optionComponentProps: { selectedOptions, onSelectedOptionsChange } },
  ref,
) {
  const innerRef = useRef<HTMLInputElement>(null);
  useImperativeHandle(ref, () => ({
    focus: () => {
      innerRef.current?.focus();
    },
  }));

  return (
    <DateFilter
      ref={ref}
      value={selectedOptions}
      onChange={onSelectedOptionsChange}
    />
  );
});

const OptionComponent = forwardRef<
  OptionComponentHandle,
  OptionComponentProps<StringOnlyFilterOptionComponentProps>
>(function OptionComponent(
  {
    childrenSearchQuery,
    onNavigateUp,
    maxHeight,
    optionComponentProps: {
      option,
      selectedOptions,
      onSelectedOptionsChange,
      onClose,
    },
  },
  ref,
) {
  const menuListRef = useRef<HTMLUListElement>(null);
  useImperativeHandle(ref, () => ({
    focus: () => {
      menuListRef.current
        ?.querySelector<HTMLElement>('[role=menuitem]')
        ?.focus();
    },
  }));
  useEffect(() => () => onClose(), [onClose]);

  const flipOption = (o2Value: string) => {
    const newValues = selectedOptions.includes(o2Value)
      ? selectedOptions.filter((v) => v !== o2Value)
      : selectedOptions.concat(o2Value);

    onSelectedOptionsChange(newValues);
  };

  const initialSelections = useRef(selectedOptions);
  const fuzzySorted = useMemo(
    () =>
      fuzzySort(childrenSearchQuery)(
        (option as StringListCategory).options || [],
        (o) => o.label,
      ).sort((a, b) => {
        const isASelected = initialSelections.current.includes(a.el.value);
        const isBSelected = initialSelections.current.includes(b.el.value);

        if (isASelected && !isBSelected && a.score >= 0) return -1;
        if (isBSelected && !isASelected && b.score >= 0) return 1;

        return b.score - a.score;
      }),
    [childrenSearchQuery, option],
  );

  return (
    <MenuList
      ref={menuListRef}
      variant="selectedMenu"
      sx={{
        maxHeight: maxHeight,
        width: 300,
      }}
    >
      <OptionsMenu
        elements={fuzzySorted}
        selectedElements={new Set(selectedOptions)}
        flipOption={(value) => flipOption(value)}
        onNavigateUp={onNavigateUp}
        onNavigateDown={() => {}} // currently just blocking navigating down from the last element
      />
    </MenuList>
  );
});
