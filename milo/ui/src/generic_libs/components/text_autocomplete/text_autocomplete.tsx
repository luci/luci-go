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

import {
  Box,
  FormControl,
  FormControlProps,
  InputAdornment,
  TextField,
  TextFieldProps,
  styled,
} from '@mui/material';
import { debounce } from 'lodash-es';
import { useState, useRef, useEffect, useMemo, ReactNode } from 'react';
import { useClickAway } from 'react-use';

import { CommitOrClear } from './commit_or_clear';
import { HasUncommittedCtx, SettersCtx } from './context';
import { OptionRow } from './option_row';
import { OptionDef } from './types';

const RootContainer = styled(Box)`
  display: inline-block;
  width: 100%;
`;

const InputContainer = styled(FormControl)`
  width: 100%;
`;

const OptionsAnchor = styled(Box)`
  position: relative;
  height: 1px;
`;

const OptionsContainer = styled(Box)(
  ({ theme }) => `
    position: absolute;
    top: 0px;
    width: 100%;
    border: 1px solid var(--divider-color);
    box-sizing: border-box;
    border-radius: 0.25rem;
    background: white;
    color: var(--active-color);
    padding: 2px;
    z-index: ${theme.zIndex.tooltip - 1};
    max-height: 200px;
    overflow-y: auto;
`,
);

const OptionTable = styled('table')`
  border-spacing: 0 1px;
  table-layout: fixed;
  width: 100%;
  word-break: break-word;
`;

export interface TextAutoCompleteProps<T> {
  readonly value: string;
  /**
   * Usually, `onValueCommit` is only called with the new value when user
   * presses `Enter` while no option is selected.
   */
  readonly onValueCommit: (newValue: string) => void;

  readonly options: readonly OptionDef<T>[];
  readonly onRequestOptionsUpdate: (value: string, cursorPos: number) => void;
  readonly renderOption: (option: OptionDef<T>) => ReactNode;
  readonly applyOption: (
    value: string,
    cursorPos: number,
    option: OptionDef<T>,
  ) => readonly [string, number];

  readonly placeholder?: string;
  /**
   * Highlight the input box for a short period of time when
   * 1. `highlightInitValue` is true when first rendered, and
   * 2. `value` is not empty when first rendered.
   */
  readonly highlightInitValue?: boolean;
  /**
   * When set to true, `onValueCommit` is also called when user an option is
   * selected.
   */
  readonly implicitCommit?: boolean;
  /**
   * The delay between when user last updated the text or moved the text cursor
   * and when `onRequestOptionsUpdate` is called. This can be used the throttle
   * option updates. Only the initial value is used. Updating this value once
   * the component is initialized has no effect.
   */
  readonly initOptionsUpdateDelayMs?: number;

  readonly slotProps?: {
    readonly formControl?: FormControlProps;
    readonly textField?: Omit<
      TextFieldProps,
      | 'inputRef'
      | 'onBlur'
      | 'onClick'
      | 'onChange'
      | 'onFocus'
      | 'onInput'
      | 'onKeyDown'
      | 'onKeyUp'
      | 'placeholder'
      | 'value'
    >;
  };
}

export function TextAutocomplete<T>({
  value,
  onValueCommit,
  options,
  onRequestOptionsUpdate,
  renderOption,
  applyOption,
  highlightInitValue = false,
  implicitCommit = false,
  initOptionsUpdateDelayMs = 200,
  placeholder = '',
  slotProps = {},
}: TextAutoCompleteProps<T>) {
  const [highlightOptionId, setHighlightOptionId] = useState<string | null>(
    null,
  );
  const [showOptions, setShowOptions] = useState(false);
  const [focused, setFocused] = useState(false);
  const [uncommittedValue, setUncommittedValue] = useState(value);

  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Keep the value in sync.
  const valueRef = useRef(value);
  const previousCommitValueRef = useRef(value);
  if (value !== valueRef.current) {
    valueRef.current = value;

    // The value provided by the parent should be treated as the source of
    // truth. If the parent provides a new value, discard the pending value by
    // syncing it with the current value.
    // However, do not discard the pending value if the new value is set by the
    // previous `onValueCommit` call. Otherwise, if an edit happens after
    // `onValueCommit` is called but before the parent component is rerendered,
    // the edit will be discarded.
    if (previousCommitValueRef.current !== value) {
      // Set the pending value in the rendering cycle so we don't render the
      // stale value when the parent triggers an update.
      setUncommittedValue(value);
    }
  }
  function commitValue() {
    onValueCommit(uncommittedValue);
    previousCommitValueRef.current = uncommittedValue;
  }

  // Apply initial highlight when filter is pre-populated.
  const initValueRef = useRef(value);
  const initHighlightInitValue = useRef(highlightInitValue);
  useEffect(() => {
    if (initHighlightInitValue.current && initValueRef.current) {
      inputRef.current!.style.setProperty('animation', 'highlight 2s');
    }
  }, []);

  // Manage options updates.
  // Do not allow delayMs to be updated so we can make the implementation
  // simpler.
  const suggestionDelayMsRef = useRef(initOptionsUpdateDelayMs);
  const requestOptionsUpdateRef = useRef(onRequestOptionsUpdate);
  requestOptionsUpdateRef.current = onRequestOptionsUpdate;
  const updateGenOptionsParams = useMemo(
    () =>
      debounce(
        () =>
          requestOptionsUpdateRef.current(
            inputRef.current!.value,
            inputRef.current!.selectionStart ?? inputRef.current!.value.length,
          ),
        suggestionDelayMsRef.current,
      ),
    [],
  );

  function toggleShowOptions(newShowOptions: boolean) {
    setShowOptions(newShowOptions);

    // When the options dropdown was toggled from hidden to show, we do not want
    // to render the old options. Request new options immediately.
    if (!showOptions && newShowOptions) {
      updateGenOptionsParams();
      updateGenOptionsParams.flush();
    }
  }

  // Handle when an option is confirmed by the user.
  function handleOptionConfirmed(option: OptionDef<T>) {
    const [newVal, newPos] = applyOption(
      uncommittedValue,
      inputRef.current!.selectionStart ?? uncommittedValue.length,
      option,
    );

    setUncommittedValue(newVal);
    inputRef.current!.value = newVal;
    inputRef.current!.setSelectionRange(newPos, newPos);
  }

  // Dismiss options when the user clicks away.
  useClickAway(containerRef, () => {
    toggleShowOptions(false);
    setHighlightOptionId(null);
  });

  const commitValueRef = useRef(commitValue);
  commitValueRef.current = commitValue;
  const setters = useMemo(
    () => ({
      commit: () => commitValueRef.current(),
      clear: () => {
        setUncommittedValue('');
        inputRef.current!.value = '';
        inputRef.current!.setSelectionRange(0, 0);
        commitValueRef.current();
      },
    }),
    [],
  );

  // Handle various keyboard shortcuts.
  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    switch (e.code) {
      case 'ArrowDown': {
        toggleShowOptions(true);
        setHighlightOptionId(
          nextSelectableOptionId(options, highlightOptionId, 'down'),
        );
        break;
      }
      case 'ArrowUp': {
        if (showOptions) {
          setHighlightOptionId(
            nextSelectableOptionId(options, highlightOptionId, 'up'),
          );
        }
        break;
      }
      case 'Escape': {
        toggleShowOptions(false);
        setHighlightOptionId(null);
        break;
      }
      case 'Enter': {
        const selectedEntry = getSelectableOption(
          options,
          selectableOptionIndex(options, highlightOptionId),
        );
        if (selectedEntry) {
          handleOptionConfirmed(selectedEntry);
        }
        if (implicitCommit || selectedEntry === null) {
          commitValue();
        }
        toggleShowOptions(false);
        setHighlightOptionId(null);
        break;
      }
      default: {
        return;
      }
    }

    e.preventDefault();
  }

  const hint =
    focused && options.length > 0
      ? showOptions
        ? 'Use ↑ and ↓ to navigate, ⏎ to confirm, esc to dismiss options'
        : 'Press ↓ or start typing to see options'
      : placeholder;

  return (
    <SettersCtx.Provider value={setters}>
      <HasUncommittedCtx.Provider value={uncommittedValue !== value}>
        <RootContainer ref={containerRef}>
          <InputContainer {...slotProps.formControl}>
            <TextField
              {...slotProps.textField}
              slotProps={{
                ...slotProps.textField?.slotProps,
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <CommitOrClear />
                    </InputAdornment>
                  ),
                  ...slotProps.textField?.slotProps?.input,
                },
              }}
              inputRef={inputRef}
              placeholder={hint}
              value={uncommittedValue}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              onKeyDown={handleKeyDown}
              onKeyUp={() => updateGenOptionsParams()}
              onChange={(e) => setUncommittedValue(e.target.value)}
              onInput={() => {
                toggleShowOptions(true);
                updateGenOptionsParams();
              }}
              onClick={() => {
                toggleShowOptions(true);
                updateGenOptionsParams();
              }}
              autoComplete={slotProps.textField?.autoComplete ?? 'off'}
            />
          </InputContainer>
          <OptionsAnchor>
            <OptionsContainer
              sx={{
                display: showOptions && options.length > 0 ? '' : 'none',
              }}
            >
              <OptionTable>
                <tbody>
                  {options.map((option) => (
                    <OptionRow
                      key={option.id}
                      def={option}
                      selected={option.id === highlightOptionId}
                      onClick={() => {
                        setHighlightOptionId(null);
                        toggleShowOptions(false);
                        handleOptionConfirmed(option);
                        if (implicitCommit) {
                          commitValue();
                        }
                        inputRef.current!.focus();
                      }}
                    >
                      {renderOption(option)}
                    </OptionRow>
                  ))}
                </tbody>
              </OptionTable>
            </OptionsContainer>
          </OptionsAnchor>
        </RootContainer>
      </HasUncommittedCtx.Provider>
    </SettersCtx.Provider>
  );
}

/**
 * Find the selectable option entry with a matching ID.
 */
function getSelectableOption<T>(
  options: readonly OptionDef<T>[],
  index: number,
): OptionDef<T> | null {
  if (index === -1) {
    return null;
  }
  const entry = options.at(index);
  return !entry || entry.unselectable ? null : entry;
}

/**
 * Find the index of the selectable option entry with a matching ID.
 */
function selectableOptionIndex<T>(
  options: readonly OptionDef<T>[],
  id: string | null,
): number {
  if (id === null) {
    return -1;
  }
  return options.findIndex((s) => !s.unselectable && s.id === id);
}

/**
 * Find the ID of the next selectable option entry.
 */
function nextSelectableOptionId<T>(
  options: readonly OptionDef<T>[],
  id: string | null,
  direction: 'up' | 'down',
): string | null {
  const selectedIndex = selectableOptionIndex(options, id);
  const normalizedSelectedIndex =
    selectedIndex === -1 && direction === 'up' ? options.length : selectedIndex;

  for (let offset = 1; offset <= options.length; ++offset) {
    const nextIndex =
      normalizedSelectedIndex + (direction === 'down' ? offset : -offset);
    const normalizedNextIndex = (nextIndex + options.length) % options.length;
    const nextEntry = options[normalizedNextIndex];
    if (!nextEntry.unselectable) {
      return nextEntry.id;
    }
  }

  return getSelectableOption(options, selectedIndex)?.id ?? null;
}
