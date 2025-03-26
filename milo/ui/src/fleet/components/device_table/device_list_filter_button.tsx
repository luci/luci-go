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
import { useCallback, useEffect, useState } from 'react';

import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { fuzzySort, fuzzySubstring } from '@/fleet/utils/fuzzy_sort';

import { FilterButton } from '../filter_dropdown/filter_button';
import {
  FilterCategoryData,
  OptionComponentProps,
} from '../filter_dropdown/filter_dropdown';
import { OptionsMenu } from '../filter_dropdown/options_menu';

export function DeviceListFilterButton({
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

  const filterCategoryDatas: FilterCategoryData<DeviceListFilterOptionComponentProps>[] =
    filterOptions.map((option) => {
      return {
        label: option.label,
        value: option.value,
        getSearchScore: (searchQuery: string) => {
          const sortedChildren = fuzzySort(searchQuery)(
            option.options,
            (o) => o.label,
          );
          const [parentScore, parentMatches] = fuzzySubstring(
            searchQuery,
            option.label,
          );
          return {
            score: Math.max(sortedChildren[0].score, parentScore),
            matches: parentMatches,
          };
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
      } as FilterCategoryData<DeviceListFilterOptionComponentProps>;
    });

  return (
    <FilterButton
      filterOptions={filterCategoryDatas}
      isLoading={isLoading}
      onApply={() => {
        onSelectedOptionsChange(tempSelectedOptions);
      }}
    />
  );
}

interface DeviceListFilterOptionComponentProps {
  option: OptionCategory;
  selectedOptions: string[];
  onSelectedOptionsChange: (newSelectedOptions: string[]) => void;
  onClose: () => void;
}

const OptionComponent = ({
  searchQuery,
  optionComponentProps: {
    option,
    selectedOptions,
    onSelectedOptionsChange,
    onClose,
  },
}: OptionComponentProps<DeviceListFilterOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  const flipOption = (o2Value: string) => {
    const newValues = selectedOptions.includes(o2Value)
      ? selectedOptions.filter((v) => v !== o2Value)
      : selectedOptions.concat(o2Value);

    onSelectedOptionsChange(newValues);
  };

  const fuzzySorted = fuzzySort(searchQuery)(option.options, (o) => o.label);

  return (
    <MenuList
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
      />
    </MenuList>
  );
};
