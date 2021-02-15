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

import { fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import sinon, { SinonSpy } from 'sinon';

import { AutoCompleteElement, Suggestion } from './auto_complete';

function simulateKeyStroke(target: EventTarget, code: string) {
  target.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, code} as KeyboardEventInit));
  target.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, code} as KeyboardEventInit));
}

const suggestions = [
  {value: 'suggestion 1', explanation: 'explanation 1'},
  {value: 'suggestion 2', explanation: 'explanation 2'},
  {value: 'suggestion 3', explanation: 'explanation 3'},
];

describe('auto_complete_test', () => {
  let autoSuggestionEle: AutoCompleteElement;
  let inputEle: HTMLInputElement;
  let suggestionSpy: SinonSpy<[Suggestion], void>;
  before(async () => {
    autoSuggestionEle = await fixture<AutoCompleteElement>(html`
      <milo-auto-complete
          .value=${'search text'}
          .placeHolder=${'Press / to search test results...'}
          .suggestions=${suggestions}
      >
      </milo-auto-complete>
    `);
    inputEle = autoSuggestionEle.shadowRoot!.querySelector('input')!;
    suggestionSpy = sinon.spy(autoSuggestionEle, 'onSuggestionSelected');
  });
  after(fixtureCleanup);

  it('should be able to select suggestion with key stokes', () => {
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    assert.strictEqual(suggestionSpy.getCall(0).args[0], suggestions[1]);
  });

  it('should reset suggestion selection when suggestions are updated', () => {
    autoSuggestionEle.suggestions = suggestions.slice(0, 2);
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    assert.strictEqual(suggestionSpy.getCall(1).args[0], suggestions[1]);

    autoSuggestionEle.suggestions = suggestions.slice(0);
    simulateKeyStroke(inputEle, 'ArrowDown');
    simulateKeyStroke(inputEle, 'Enter');
    assert.strictEqual(suggestionSpy.getCall(2).args[0], suggestions[0]);
  });
});
