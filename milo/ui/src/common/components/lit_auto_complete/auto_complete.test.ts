// Copyright 2021 The LUCI Authors.
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

import { fixture } from '@open-wc/testing-helpers';
import { html } from 'lit';

import './auto_complete';
import {
  AutoCompleteElement,
  Suggestion,
  SuggestionEntry,
} from './auto_complete';

function simulateKeyStroke(target: EventTarget, code: string) {
  target.dispatchEvent(
    new KeyboardEvent('keydown', {
      bubbles: true,
      composed: true,
      code,
    } as KeyboardEventInit),
  );
  target.dispatchEvent(
    new KeyboardEvent('keyup', {
      bubbles: true,
      composed: true,
      code,
    } as KeyboardEventInit),
  );
}

const suggestions: Suggestion[] = [
  { value: 'suggestion 1', explanation: 'explanation 1' },
  { value: 'suggestion 2', explanation: 'explanation 2' },
  { value: 'suggestion 3', explanation: 'explanation 3' },
  { isHeader: true, display: 'header' },
  { value: 'suggestion 4', explanation: 'explanation 4' },
  { value: 'suggestion 5', explanation: 'explanation 5' },
  { isHeader: true, display: 'header' },
];

describe('AuthComplete', () => {
  let autoSuggestionEle: AutoCompleteElement;
  let inputEle: HTMLInputElement;
  let suggestionSpy: jest.SpiedFunction<(_suggestion: SuggestionEntry) => void>;
  let completeSpy: jest.SpiedFunction<() => void>;

  beforeAll(async () => {
    autoSuggestionEle = await fixture<AutoCompleteElement>(html`
      <milo-auto-complete
        .value=${'search text'}
        .placeHolder=${'Press / to search test results...'}
        .suggestions=${suggestions}
      >
      </milo-auto-complete>
    `);
    inputEle = autoSuggestionEle.shadowRoot!.querySelector('input')!;
    suggestionSpy = jest.spyOn(autoSuggestionEle, 'onSuggestionSelected');
    completeSpy = jest.spyOn(autoSuggestionEle, 'onComplete');
  });

  test('should be able to select suggestion with key strokes', () => {
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(suggestions[1]);
    expect(completeSpy.mock.calls.length).toStrictEqual(0);
  });

  test('should reset suggestion selection when suggestions are updated', () => {
    const suggestionSpy = jest.spyOn(autoSuggestionEle, 'onSuggestionSelected');
    const completeSpy = jest.spyOn(autoSuggestionEle, 'onComplete');
    autoSuggestionEle.suggestions = suggestions.slice(0, 2);
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(suggestions[1]);

    autoSuggestionEle.suggestions = suggestions.slice(0);
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(suggestions[0]);

    expect(completeSpy.mock.calls.length).toStrictEqual(0);
  });

  test('should skip suggestion headers when selecting with key strokes', () => {
    autoSuggestionEle.suggestions = suggestions.slice();
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(suggestions[4]);

    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowUp');
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(suggestions[2]);

    expect(completeSpy.mock.calls.length).toStrictEqual(0);
  });

  test('should not navigate beyond boundary', () => {
    autoSuggestionEle.suggestions = suggestions.slice();
    for (let i = 0; i < suggestions.length * 2; ++i) {
      simulateKeyStroke(inputEle, 'ArrowDown');
    }
    simulateKeyStroke(inputEle, 'Enter');
    const lastSelectableSuggestion = suggestions
      .slice()
      .reverse()
      .find((s) => !s.isHeader);
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(
      lastSelectableSuggestion,
    );

    const firstSelectableSuggestion = suggestions.find((s) => !s.isHeader);
    simulateKeyStroke(inputEle, 'ArrowDown');
    for (let i = 0; i < suggestions.length * 2; ++i) {
      simulateKeyStroke(inputEle, 'ArrowUp');
    }
    simulateKeyStroke(inputEle, 'Enter');
    expect(suggestionSpy.mock.lastCall?.[0]).toStrictEqual(
      firstSelectableSuggestion,
    );

    expect(completeSpy.mock.calls.length).toStrictEqual(0);
  });

  test('should call onComplete when user hit enter with completed query', () => {
    autoSuggestionEle.value = 'search text ';
    simulateKeyStroke(inputEle, 'Enter');
    expect(completeSpy.mock.calls.length).toStrictEqual(1);
  });
});
