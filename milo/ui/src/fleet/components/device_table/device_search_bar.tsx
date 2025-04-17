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

import { useMemo, useState } from 'react';

import { colors } from '@/fleet/theme/colors';
import { OptionValue } from '@/fleet/types/option';
import {
  TextAutocomplete,
  OptionDef,
} from '@/generic_libs/components/text_autocomplete';

import { HighlightCharacter } from '../highlight_character';

const NoMatchFound: OptionDef<string> = {
  id: '',
  value: 'No matching devices found',
  unselectable: true,
};

const findFirstMatchingIndex = (
  options: OptionDef<string>[],
  startFrom: number,
  query: string,
): number => {
  if (startFrom < 0) {
    return startFrom;
  }

  for (let i = startFrom; i < options.length; i++) {
    const deviceID = options[i].id;
    if (deviceID.startsWith(query)) {
      return i;
    }
    // Options are sorted so we can fail fast.
    if (deviceID.localeCompare(query) > 0) {
      break;
    }
  }

  return -1;
};

const getHighlightIndexes = (id: string, query: string): number[] => {
  const idx = id.indexOf(query);
  return idx === -1 ? [] : Array.from(Array(idx + query.length).keys());
};

export const DeviceSearchBar = ({
  options,
  applySelectedOption,
  numSuggestions = 10,
}: {
  options: OptionValue[];
  applySelectedOption: (selectedOptionId: string) => void;
  numSuggestions?: number;
}) => {
  const [query, setQuery] = useState<string>('');
  const [selectedOption, setSelectedOption] = useState<string>('');
  // Index of the first matching device in the sorted options list.
  const [firstMatchIdx, setFirstMatchIdx] = useState<number>(-1);

  const sortedOptions = useMemo(() => {
    const optionDefs = options.map((opt) => ({
      id: opt.value,
      value: opt.label,
    }));
    optionDefs.sort((a, b) => a.id.localeCompare(b.id));
    return optionDefs;
  }, [options]);

  const suggestions = useMemo(() => {
    if (query === '') {
      return [];
    }
    if (firstMatchIdx === -1) {
      return [NoMatchFound];
    }
    return sortedOptions.slice(firstMatchIdx, firstMatchIdx + numSuggestions);
  }, [query, firstMatchIdx, sortedOptions, numSuggestions]);

  const updateOptions = (newQuery: string) => {
    let startFrom: number;
    if (newQuery === '') {
      startFrom = -1;
    } else if (query && newQuery.startsWith(query)) {
      // Search from previous top index since prefixes match.
      startFrom = firstMatchIdx;
    } else {
      startFrom = 0;
    }
    setQuery(newQuery);
    setFirstMatchIdx(
      findFirstMatchingIndex(sortedOptions, startFrom, newQuery),
    );
  };

  return (
    <TextAutocomplete
      value={selectedOption}
      sx={{
        width: 350,
        '& .options-container': { maxHeight: '100%' },
        '& .options-table': {
          fontSize: '16px',
          '& td': { padding: '8px 4px', color: colors.grey[800] },
        },
      }}
      options={suggestions}
      onValueCommit={setSelectedOption}
      onRequestOptionsUpdate={updateOptions}
      placeholder="Search devices by ID"
      applyOption={(value, cursorPos, option) => {
        if (option !== null && option !== NoMatchFound) {
          setSelectedOption(option.value);
          applySelectedOption(option.id);
          return [option.value, option.value.length];
        }
        return [value, cursorPos];
      }}
      renderOption={(option) => (
        <td>
          <HighlightCharacter
            highlightIndexes={
              option === NoMatchFound
                ? []
                : getHighlightIndexes(option.value, query)
            }
          >
            {option.value}
          </HighlightCharacter>
        </td>
      )}
      slotProps={{
        textField: {
          slotProps: {
            input: {
              inputProps: {
                size: 'small',
              },
            },
          },
        },
      }}
    />
  );
};
