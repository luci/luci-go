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
 * 1. For functions that may throw, wrap them in  in reportError or
 *    reportErrorAsync, this converts error to error events.
 * 2. Decorate the element that should handle the error events with
 *    @errorHandler().
 *
 * For example:
 * ```typescript
 * @customElement('some-component-that-may-throw')
 * export class SomeComponentThatMayThrow extends LitElement {
 *   protected render = reportError(this, () => {
 *     // Rendering code goes here.
 *   });
 * }
 *
 * @customElement('parent-element')
 * @errorHandler()
 * export class ParentElement extends LitElement {
 *   protected render() {
 *     return html`
 *       <some-component-that-may-throw></some-component-that-may-throw>
 *     `;
 *   }
 * }
 * ```
 */

import { html, LitElement } from 'lit';

import { toError } from '@/generic_libs/tools/utils';
import { Constructor } from '@/generic_libs/types';

/**
 * Wraps the specified fn. Whenever fn throws an error, dispatches an error
 * event from the element.
 *
 * @param ele the element where the error event should be dispatched from/
 * @param fn the fn of which the error will be reported as an error event.
 * @param fallbackFn the fallback function invoked when fn throws an error to
 *     recover from the error
 * @returns the return value of fn, or the return value of fallbackFn if fn
 *     thrown an error.
 * @throws the error thrown by fn, or the error thrown by fallbackFn if
 *     fallbackFn is specified.
 */
export function reportError<T extends unknown[], V>(
  ele: Element,
  fn: (...params: T) => V,
  fallbackFn?: (err: unknown, ...params: T) => V,
): (...params: T) => V {
  return (...params: T) => {
    try {
      return fn(...params);
    } catch (e) {
      const err = toError(e);
      ele.dispatchEvent(
        new ErrorEvent('error', {
          error: err,
          message: err.message,
          composed: true,
          bubbles: true,
          cancelable: true,
        }),
      );
      if (fallbackFn) {
        return fallbackFn(e, ...params);
      }
      throw e;
    }
  };
}

/**
 * A special case of reportError that is intended to be used to wrap
 * LitElement.render.
 *
 * By default, fallback to render the error message in a <pre>.
 */
export function reportRenderError(
  ele: Element,
  fn: () => unknown,
  fallbackFn = (_err: unknown): unknown => '',
): () => unknown {
  return reportError(ele, fn, fallbackFn);
}

/**
 * Similar to reportError but support async functions.
 *
 * See the documentation of reportError for details.
 */
export function reportErrorAsync<T extends unknown[], V>(
  ele: Element,
  fn: (...params: T) => Promise<V>,
  fallbackFn?: (err: unknown, ...params: T) => Promise<V>,
): (...params: T) => Promise<V> {
  return async (...params: T) => {
    try {
      // Don't elide await here or non-immediate errors won't be cached.
      return await fn(...params);
    } catch (e) {
      const err = toError(e);
      ele.dispatchEvent(
        new ErrorEvent('error', {
          error: err,
          message: err.message,
          composed: true,
          bubbles: true,
          cancelable: true,
        }),
      );
      if (fallbackFn) {
        return fallbackFn(e, ...params);
      }
      throw e;
    }
  };
}

/**
 * Specifies how the error should be handled.
 *
 * Return true to instruct the error handler element to render the error
 * message.
 */
export type OnError<T extends LitElement> = (
  err: ErrorEvent,
  ele: T,
) => boolean;

/**
 * An OnError function that stops the error from propagating. If the error
 * message is not empty, instructs the error handler element to render the
 * error.
 */
export function handleLocally<T extends LitElement>(
  err: ErrorEvent,
  _ele: T,
): boolean {
  err.stopPropagation();
  return err.message !== '';
}

/**
 * A OnError function that stops the error from propagating, dispatches a new
 * error event on the parent element with the same error but with an empty error
 * message. Ancestor error handlers can use event.preventDefault() to signal
 * that the error is recoverable. If the error message is not empty, and
 * event.preventDefault() is not called, instructs the error handler element to
 * render the error.
 */
export function forwardWithoutMsg<T extends LitElement>(
  err: ErrorEvent,
  ele: T,
): boolean {
  err.stopPropagation();
  const event = new ErrorEvent('error', {
    error: err.error,
    message: '',
    bubbles: true,
    composed: true,
    cancelable: true,
  });

  // If the event is canceled, that means the error can be recovered.
  const canceled = !(ele.parentNode?.dispatchEvent(event) ?? true);
  if (canceled) {
    err.preventDefault();
    return false;
  }

  return err.message !== '';
}

/**
 * Specifies how the error should be rendered.
 */
export type RenderErrorFn<T extends LitElement> = (
  err: ErrorEvent,
  ele: T,
) => unknown;

/**
 * A RenderErrorFn that renders the error message in a <pre> with grey
 * background.
 */
export function renderErrorInPre<T extends LitElement>(
  err: ErrorEvent,
  _ele: T,
): unknown {
  return html`
    <pre
      style="
        background-color: var(--block-background-color);
        padding: 5px;
        margin: 8px 16px;
        white-space: pre-wrap;
        overflow-wrap: break-word;
      "
    >
An error occurred:
${err.message}</pre
    >
  `;
}

/**
 * Renders the error message when there's an error event.
 */
export function errorHandler<T extends LitElement>(
  onError: OnError<T> = handleLocally,
  renderError: RenderErrorFn<T> = renderErrorInPre,
) {
  return function consumerMixin<C extends Constructor<T>>(cls: C) {
    const errorSymbol = Symbol('error');
    const errorListenerSymbol = Symbol('errorListener');

    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class ErrorHandler extends (cls as Constructor<LitElement>) {
      [errorSymbol]: ErrorEvent | null = null;
      [errorListenerSymbol] = (_e: ErrorEvent) => {
        /* implementation will be provided in `connectedCallback` */
      };

      constructor() {
        super();

        // Overrides the render function to render the error message when
        // there's an error.
        //
        // We can't use the usual overriding syntax, because that doesn't work
        // when the target function is a property function.
        // The render function is likely converted to property function in the
        // parent class with a syntax like below.
        //
        // protected render = reportRenderError(this, () => {...});
        const superRender = this.render;
        this.render = function () {
          return this[errorSymbol] === null
            ? superRender.bind(this)()
            : renderError(this[errorSymbol]!, this as LitElement as T);
        };
      }

      connectedCallback() {
        super.connectedCallback();
        this[errorListenerSymbol] = (e) => {
          if (onError(e, this as LitElement as T)) {
            this[errorSymbol] = e;

            // Requesting update immediately may cause the request to be
            // ignored when the element in the middle of an update.
            // Schedule an update instead.
            this.updateComplete.then(() => this.requestUpdate());
          } else {
            this[errorSymbol] = null;
          }
        };
        this.addEventListener('error', this[errorListenerSymbol]);
      }

      disconnectedCallback() {
        this.removeEventListener('error', this[errorListenerSymbol]);
        super.disconnectedCallback();
      }
    }
    // Recover the type information that was lost in the down-casting above.
    return ErrorHandler as Constructor<LitElement> as C;
  };
}
