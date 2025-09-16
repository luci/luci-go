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

import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

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
  const sortedFilterOptions = useMemo(
    () => elevateSelectedFiltersToTheTop(filterOptions, selectedOptions),
    [filterOptions, selectedOptions],
  );

  const [tempSelectedOptions, setTempSelectedOptions] =
    useState<SelectedOptions>(selectedOptions);

  useEffect(() => {
    setTempSelectedOptions(selectedOptions);
  }, [selectedOptions]);

  const clearSelections = useCallback(() => {
    setTempSelectedOptions(selectedOptions);
  }, [setTempSelectedOptions, selectedOptions]);

  const filterCategoryDatas: FilterCategoryData<StringOnlyFilterOptionComponentProps>[] =
    sortedFilterOptions.map((option) => {
      return {
        label: option.label,
        value: option.value,
        getChildrenSearchScore: (childrenSearchQuery: string) => {
          const sortedChildren = fuzzySort(childrenSearchQuery)(
            option.options,
            (o) => o.label,
          );
          return sortedChildren[0]?.score;
        },
        optionsComponent: OptionComponent,
        optionsComponentProps: {
          option: option,
          selectedOptions: tempSelectedOptions[option.value] ?? [],
          onSelectedOptionsChange: (newSelectedOptions) =>
            setTempSelectedOptions({
              ...tempSelectedOptions,
              [option.value]: newSelectedOptions,
            }),
          onClose: clearSelections,
        },
      } as FilterCategoryData<StringOnlyFilterOptionComponentProps>;
    });

  const getChipLabel = (filterCategory: FilterCategoryData<unknown>) => {
    const selectedValuesValues = selectedOptions[filterCategory.value];

    const filterOption = filterOptions.find(
      (opt) => opt.value === filterCategory.value,
    );
    if (!filterOption) {
      return '';
    }

    const optionLabels = selectedValuesValues.map(
      (v) => filterOption.options.find((o) => o.value === v)?.label ?? v,
    );

    return `${selectedOptions[filterCategory.value].length ?? 0} | [ ${filterCategory.label} ]: ${optionLabels.join(
      ', ',
    )}`;
  };

  return (
    <FilterBar
      filterCategoryDatas={filterCategoryDatas}
      selectedOptions={Object.keys(selectedOptions)}
      onApply={() => onSelectedOptionsChange(tempSelectedOptions)}
      getChipLabel={(o) => getChipLabel(o as FilterCategoryData<unknown>)}
      onChipDeleted={(o) => {
        delete selectedOptions[o.value];
        onSelectedOptionsChange({
          ...selectedOptions,
          [o.value]: [],
        });
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

function elevateSelectedFiltersToTheTop(
  filterOptions: OptionCategory[],
  selectedOptions: SelectedOptions,
): OptionCategory[] {
  // Unselected filters are also considered for reorganizing,
  // as they are included in the selectedOptions with an empty array.
  return filterOptions.map((filter) => {
    if (filter.value in selectedOptions) {
      filter.options.sort((a, b) => {
        const aIsSelected = selectedOptions[filter.value].includes(a.value);
        const bIsSelected = selectedOptions[filter.value].includes(b.value);
        if (aIsSelected && !bIsSelected) return -1;
        if (!aIsSelected && bIsSelected) return 1;

        return a.value.localeCompare(b.value);
      });
    }

    return filter;
  });
}

const OptionComponent = forwardRef<
  OptionComponentHandle,
  OptionComponentProps<StringOnlyFilterOptionComponentProps>
>(function OptionComponent(
  {
    childrenSearchQuery,
    onNavigateUp,
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

  const fuzzySorted = fuzzySort(childrenSearchQuery)(
    option.options,
    (o) => o.label,
  );

  return (
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
        selectedElements={new Set(selectedOptions)}
        flipOption={(value) => flipOption(value)}
        onNavigateUp={onNavigateUp}
        onNavigateDown={() => {}} // currently just blocking navigating down from the last element
      />
    </MenuList>
  );
});
