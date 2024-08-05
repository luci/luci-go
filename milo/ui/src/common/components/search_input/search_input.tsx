// Copyright 2023 The LUCI Authors.
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

import Search from '@mui/icons-material/Search';
import FormControl from '@mui/material/FormControl';
import InputAdornment from '@mui/material/InputAdornment';
import TextField, {
  TextFieldProps,
  TextFieldVariants,
} from '@mui/material/TextField';
import { useRef, useState } from 'react';
import { useDebounce, useKey, useLatest } from 'react-use';

export interface SearchInputProps<Variant extends TextFieldVariants>
  extends Omit<TextFieldProps, 'variant' | 'onChange' | 'inputRef' | 'value'> {
  readonly variant?: Variant;
  readonly value: string;
  /**
   * Invoked with the new value `initDelayMs`ms after the user stops editing.
   */
  readonly onValueChange: (newValue: string) => void;
  /**
   * When defined, focus on the input box when the user presses the key.
   */
  readonly focusShortcut?: string;
  /**
   * `onValueChange` will only be invoked `initDelayMs`ms after the user stops
   * editing. Only the initial value is respected. Updating this value has no
   * effect. Defaults to 0ms.
   */
  readonly initDelayMs?: number;
}

export function SearchInput<Variant extends TextFieldVariants>({
  value,
  focusShortcut,
  initDelayMs = 0,
  onValueChange,
  InputProps,
  inputProps,
  sx,
  ...props
}: SearchInputProps<Variant>) {
  const [pendingValue, setPendingValue] = useState(value);
  const valueRef = useRef(value);
  const previousUpdateValueRef = useRef(value);

  const inputRef = useRef<HTMLInputElement>(null);
  useKey(
    (e) => {
      if (e.key !== focusShortcut) {
        return false;
      }

      // Get the tag name of the event target in case its enclosed in shadow
      // DOM.
      const tagName =
        (e.composedPath()[0] as Partial<HTMLElement>).tagName || '';
      // Do not react to events from input related elements.
      return !['INPUT', 'SELECT', 'TEXTAREA'].includes(tagName);
    },
    // Set timeout so that the input box do not record the key that triggered
    // the focus.
    () => setTimeout(() => inputRef.current?.focus(), 0),
    {},
    [focusShortcut],
  );

  if (value !== valueRef.current) {
    valueRef.current = value;

    // The value provided by the parent should be treated as the source of
    // truth. If the parent provides a new value, discard the pending value by
    // syncing it with the current value.
    // However, do not discard the pending value if the new value is set by the
    // previous `onValueChange` call. Otherwise, if an edit happens after
    // `onValueChange` is called but before the parent component is rerendered,
    // the edit will be discarded. This can happen when the user makes an edit
    // exactly `initDelayMs` milliseconds after the previous edit.
    if (previousUpdateValueRef.current !== value) {
      // Set the pending value in the rendering cycle so we don't render the
      // stale value when the parent triggers an update.
      setPendingValue(value);
    }
  }

  // Do not allow delayMs to be updated so we can make the implementation
  // simpler.
  const delayMs = useRef(initDelayMs).current;

  // Store the function in ref so we don't need to keep resetting debounce when
  // the callback is updated (especially when its not referentially stable).
  const onValueChangeRef = useLatest(onValueChange);

  useDebounce(
    () => {
      // No point calling the callback when the pending value is the same as the
      // actual value. This can happen when the pending value is set by the
      // parent, or when user discarded their edit.
      if (valueRef.current === pendingValue) {
        return;
      }
      previousUpdateValueRef.current = pendingValue;
      onValueChangeRef.current(pendingValue);
    },
    delayMs,
    [pendingValue],
  );

  return (
    <FormControl sx={{ width: '100%', ...sx }}>
      <TextField
        inputProps={{
          'data-testid': 'search-input',
          ...inputProps,
        }}
        size="small"
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <Search />
            </InputAdornment>
          ),
          ...InputProps,
        }}
        {...props}
        // Needed to ensure the component functions correctly.
        // Don't allow them to be overridden.
        value={pendingValue}
        onChange={(e) => setPendingValue(e.target.value)}
        inputRef={inputRef}
      />
    </FormControl>
  );
}
