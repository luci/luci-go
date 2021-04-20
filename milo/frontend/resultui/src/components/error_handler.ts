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

/**
 * @fileoverview
 * This files contains utilities for handling errors originated from an element.
 *
 * To use this component, you need to
 * 1. Wrap the element that may throw an error in <milo-error-handler />. The
 *   element doesn't need to be the direct child of <milo-error-handler /> and
 *   can be in shadow DOM.
 * 2. Wrap the function that may throw in reportError or reportErrorAsync.
 *
 * For example:
 * ```typescript
 * @customElement('some-component-that-may-throw')
 * export class SomeComponentThatMayThrow extends LitElement {
 *   protected render = reportError.bind(this)(() => {
 *     // Rendering code goes here.
 *   });
 * }
 *
 * @customElement('parent-element')
 * export class ParentElement extends LitElement {
 *   protected render() {
 *     return html`
 *       <milo-error-handler>
 *         <some-component-that-may-throw></some-component-that-may-throw>
 *       </milo-error-handler>
 *     `;
 *   }
 * }
 * ```
 *
 * You can also specify a fallback function:
 * ```typescript
 * @customElement('some-component-that-may-throw')
 * export class SomeComponentThatMayThrow extends LitElement {
 *   protected render = reportError.bind(this)(
 *     () => {
 *     // Rendering code goes here.
 *     },
 *     () => html`there was an error`
 *   );
 * }
 * ```
 *
 * An intercept function can be provided for custom error handling.
 * ```
 * @customElement('parent-element')
 * export class ParentElement extends LitElement {
 *   protected render() {
 *     return html`
 *       <milo-error-handler
 *         .intercept=${(e: ErrorEvent) => {
 *            // Handle errors.
 *            if (recoveredFromTheError) {
 *              return true;
 *            }
 *            return false;
 *         }}
 *       >
 *         <some-component-that-may-throw></some-component-that-may-throw>
 *       </milo-error-handler>
 *     `;
 *   }
 * }
 * ```
 *
 * If an element have multiple <milo-error-handler> ancestors, only the closest
 * <milo-error-handler> ancestor will receive the error event.
 */

import { css, customElement, html, LitElement, property } from 'lit-element';

import commonStyle from '../styles/common_style.css';

/**
 * Wraps the specified fn. Whenever fn throws, dispatches an error event from
 * `this` element.
 */
export function reportError<T extends unknown[], V>(
  this: Element,
  fn: (...params: T) => V,
  fallbackFn?: (err: unknown, ...params: T) => V
): (...params: T) => V {
  return (...params: T) => {
    try {
      return fn(...params);
    } catch (e) {
      this.dispatchEvent(
        new ErrorEvent('error', {
          error: e,
          message: e.toString(),
          composed: true,
          bubbles: true,
        })
      );
      if (fallbackFn) {
        return fallbackFn(e, ...params);
      }
      throw e;
    }
  };
}

/**
 * Wraps the specified async fn. Whenever fn throws, dispatches an error event
 * from `this` element.
 */
export function reportErrorAsync<T extends unknown[], V>(
  this: Element,
  fn: (...params: T) => Promise<V>,
  fallbackFn?: (err: unknown, ...params: T) => Promise<V>
): (...params: T) => Promise<V> {
  return async (...params: T) => {
    try {
      // Don't elide await here or immediate errors won't be cached.
      return await fn(...params);
    } catch (e) {
      this.dispatchEvent(
        new ErrorEvent('error', {
          error: e,
          message: e.toString(),
          composed: true,
          bubbles: true,
        })
      );
      if (fallbackFn) {
        return fallbackFn(e, ...params);
      }
      throw e;
    }
  };
}

/**
 * Handles error events from the decedents.
 */
@customElement('milo-error-handler')
export class ErrorHandlerElement extends LitElement {
  @property() errorMsg: string | null = null;

  /**
   * Called before showing error messages. Return null to ignore the error.
   */
  intercept = (e: ErrorEvent): ErrorEvent | null => e;

  private errorHandler = (event: ErrorEvent) => {
    event.stopPropagation();
    const e = this.intercept(event);
    this.errorMsg = e?.message ?? null;
  };

  connectedCallback() {
    super.connectedCallback();
    // Use the built-in error event.
    // This can catch errors reported by other ways.
    // Also allows errors to be handled by custom error event handler without
    // mounting this element.
    this.addEventListener('error', this.errorHandler);
  }

  disconnectedCallback() {
    this.removeEventListener('error', this.errorHandler);
    super.disconnectedCallback();
  }

  protected render() {
    return this.errorMsg === null
      ? html`<slot></slot>`
      : html`
          <div id="error-label">An error occurred:</div>
          <pre id="error-message">${this.errorMsg}</pre>
        `;
  }

  static styles = [
    commonStyle,
    css`
      #error-label {
        margin: 8px 16px;
      }

      #error-message {
        white-space: pre-wrap;
        margin: 8px 16px;
        background-color: var(--block-background-color);
        padding: 5px;
      }
    `,
  ];
}
