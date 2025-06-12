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

import { useEffect, useState } from 'react';

import {
  OptionCategory,
  OptionValue,
  SelectedOptions,
} from '@/fleet/types/option';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { MenuSkeleton } from '../filter_dropdown/menu_skeleton';
import { OptionsMenu } from '../filter_dropdown/options_menu';
import { SelectedChip } from '../filter_dropdown/selected_chip';
import { DIMENSION_SEPARATOR } from '../table/dimensions';

export const DeviceListSelectedChip = ({
  filterOptions,
  selectedOptions,
  isLoading,
  onSelectedOptionsChange,
  optionKey,
  optionValues,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  isLoading: boolean;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  optionKey: string;
  optionValues: string[];
}) => {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(selectedOptions);

  useEffect(() => {
    setTempSelectedOptions(selectedOptions);
  }, [selectedOptions]);

  const getChipLabel = () => {
    const filterOption = filterOptions.find((opt) => opt.value === optionKey);
    if (!filterOption) {
      return '';
    }

    const label = filterOption.label ?? '';

    return `${selectedOptions[optionKey].length ?? 0} | [ ${label} ]: ${optionValues
      .map((o) => filterOption.options.find((x) => x.value === o)?.label ?? o)
      .join(DIMENSION_SEPARATOR)}`;
  };

  return (
    <SelectedChip
      dropdownContent={
        isLoading ? (
          <MenuSkeleton
            itemCount={Math.min(filterOptions.length, 30)}
            maxHeight={200}
          />
        ) : (
          <OptionsMenu
            elements={
              filterOptions
                .find((opt) => opt.value === optionKey)
                ?.options.map(
                  (o) =>
                    ({
                      el: {
                        label: o.label,
                        value: o.value,
                      },
                      matches: [], // No search query here, so no matches to highlight
                      score: 1, // Score is not used for sorting here, items are as-is
                    }) as SortedElement<OptionValue>,
                ) ?? []
            }
            selectedElements={new Set(tempSelectedOptions[optionKey] ?? [])}
            flipOption={(selectedValue: string) => {
              const currentValues = tempSelectedOptions[optionKey] ?? [];
              if (currentValues.includes(selectedValue)) {
                setTempSelectedOptions((prev) => ({
                  ...prev,
                  [optionKey]: currentValues.filter((x) => x !== selectedValue),
                }));
              } else {
                setTempSelectedOptions((prev) => ({
                  ...prev,
                  [optionKey]: [...currentValues, selectedValue],
                }));
              }
            }}
          />
        )
      }
      label={getChipLabel()}
      onApply={() => {
        onSelectedOptionsChange(tempSelectedOptions);
      }}
      onDelete={() => {
        onSelectedOptionsChange({
          ...selectedOptions,
          [optionKey]: [],
        });
      }}
    />
  );
};
