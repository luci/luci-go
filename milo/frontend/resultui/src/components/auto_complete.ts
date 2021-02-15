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

import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { styleMap } from 'lit-html/directives/style-map';
import { observable, reaction } from 'mobx';

export interface Suggestion {
  readonly value: string;
  readonly explanation: string;
}

/**
 * An input box that supports auto-complete dropdown.
 */
@customElement('milo-auto-complete')
export class AutoCompleteElement extends MobxLitElement {
  @observable.ref value!: string;
  @observable.ref placeHolder = '';
  @observable.ref suggestions: readonly Suggestion[] = [];

  onValueUpdate = (_newVal: string) => {};
  onSuggestionSelected = (_suggestion: Suggestion) => {};

  focus() {
    this.searchBox.focus();
  }

  // -1 means nothing is selected.
  @observable.ref private selectedIndex = -1;
  @observable.ref private showSuggestions = true;

  private get searchBox() {
    return this.shadowRoot!.getElementById('search-box')!;
  }
  private get dropdownContainer() {
    return this.shadowRoot!.getElementById('dropdown-container')!;
  }

  protected updated() {
    this.shadowRoot!.querySelector('.dropdown-item.selected')?.scrollIntoView({'block': 'nearest'});
  }

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();

    // Reset suggestion state when suggestions are updated.
    this.disposer = reaction(
      () => this.suggestions,
      () => {
        this.selectedIndex = -1;
        this.showSuggestions = true;
      },
    );

    document.addEventListener('click', this.externalClickHandler);
  }

  disconnectedCallback() {
    document.removeEventListener('click', this.externalClickHandler);
    this.disposer();
    super.disconnectedCallback();
  }

  private externalClickHandler = (e: MouseEvent) => {
    // If user clicks on other elements, dismiss the dropdown.
    if (!e.composedPath().some((t) => t === this.searchBox || t === this.dropdownContainer)) {
      this.showSuggestions = false;
    }
  }

  protected render() {
    return html`
      <input
        id="search-box"
        placeholder=${this.placeHolder}
        .value=${this.value}
        @input=${(e: InputEvent) => this.onValueUpdate((e.target as HTMLInputElement).value)}
        @keydown=${(e: KeyboardEvent) => {
          switch (e.code) {
            case 'ArrowDown':
              if (this.selectedIndex < this.suggestions.length - 1) {
                this.selectedIndex++;
              }
              break;
            case 'ArrowUp':
              if (this.selectedIndex > 0) {
                this.selectedIndex--;
              }
              break;
            case 'Escape':
              this.showSuggestions = false;
              break;
            case 'Enter':
              if (this.selectedIndex !== -1) {
                this.onSuggestionSelected(this.suggestions[this.selectedIndex]);
              }
              break;
            default:
              break;
          }
        }}
      >
      <div id="dropdown-container" style=${styleMap({display: this.showSuggestions && this.suggestions.length > 0 ? '' : 'none'})}>
        <table id="dropdown">
          ${this.suggestions.map((suggestion, i) => html`
            <tr
              class=${classMap({'dropdown-item': true, 'selected': i === this.selectedIndex})}
              @mouseover=${() => this.selectedIndex = i}
              @click=${() => {
                this.onSuggestionSelected(this.suggestions[this.selectedIndex]);
                this.focus();
              }}
            >
              <td>${suggestion.value}</td>
              <td>${suggestion.explanation}</td>
            </tr>
          `)}
        </table>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
      position: relative;
    }

    #search-box {
      display: inline-block;
      width: 350px;
      padding: .3rem .5rem;
      font-size: 1rem;
      color: var(--light-text-color);
      background-clip: padding-box;
      border: 1px solid var(--divider-color);
      border-radius: .25rem;
      transition: border-color .15s ease-in-out,box-shadow .15s ease-in-out;
      text-overflow: ellipsis;
    }

    #dropdown-container {
      position: absolute;
      border: 1px solid var(--divider-color);
      background: white;
      color: var(--active-color);
      padding: 2px;
      z-index: 999;
      max-height: 200px;
      overflow-y: auto;
    }
    #dropdown {
      border-spacing: 0 1px;
    }

    .dropdown-item>td {
      white-space: nowrap;
      overflow: hidden;
    }
    .dropdown-item>td:first-child {
      padding-right: 50px;
    }
    .dropdown-item.selected {
      border-color: var(--light-active-color);
      background-color: var(--light-active-color);
    }
  `;
}
