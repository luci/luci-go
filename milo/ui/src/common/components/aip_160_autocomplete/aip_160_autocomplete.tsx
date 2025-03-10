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

import { useState } from 'react';

import {
  TextAutocomplete,
  TextAutocompleteProps,
} from '@/generic_libs/components/text_autocomplete';

import { useSuggestions } from './suggestion';
import { FieldDef } from './types';

export interface Aip160Autocomplete
  extends Omit<
    TextAutocompleteProps<unknown>,
    'options' | 'onRequestOptionsUpdate' | 'renderOption' | 'applyOption'
  > {
  readonly schema: FieldDef;
}

export function Aip160Autocomplete({ schema, ...props }: Aip160Autocomplete) {
  const [genOptionsValue, setGenOptionsValue] = useState(props.value);
  const [genOptionsCursorPos, setGenOptionsCursorPos] = useState(
    props.value.length,
  );

  const options = useSuggestions(schema, genOptionsValue, genOptionsCursorPos);

  return (
    <TextAutocomplete
      {...props}
      options={options}
      onRequestOptionsUpdate={(value, cursorPos) => {
        setGenOptionsValue(value);
        setGenOptionsCursorPos(cursorPos);
      }}
      renderOption={(option) => (
        <>
          <td colSpan={option.value.explanation ? 1 : 2}>
            {option.value.display || option.value.text}
          </td>
          {option.value.explanation && <td>{option.value.explanation}</td>}
        </>
      )}
      applyOption={(_prev, _pos, option) => option.value.apply()}
    />
  );
}
