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

/**
 * Mocks DOM properties for virtualized lists.
 *
 * In the test environment (JSDOM), DOM elements don't have a real layout
 * engine, so properties like `offsetHeight` and `offsetWidth` are 0. This can
 * cause virtualization logic to get into a long-running or infinite loop,
 * leading to test timeouts.
 *
 * This function mocks these properties to provide realistic values, allowing
 * virtualized components (like MUI's DataGrid or Menu) to render correctly
 * and quickly in tests.
 *
 * It returns a cleanup function that should be called in `afterEach` to
 * restore the original DOM properties and prevent tests from interfering with
 * each other.
 *
 * @returns A cleanup function to restore the original DOM properties.
 */
export const mockVirtualizedListDomProperties = () => {
  const originalOffsetHeight = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    'offsetHeight',
  );
  const originalOffsetWidth = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    'offsetWidth',
  );
  const originalGetBoundingClientRect = Element.prototype.getBoundingClientRect;

  Object.defineProperties(HTMLElement.prototype, {
    offsetHeight: { get: () => 32, configurable: true },
    offsetWidth: { get: () => 120, configurable: true },
  });
  Element.prototype.getBoundingClientRect = jest.fn(() => ({
    width: 120,
    height: 32,
    top: 0,
    left: 0,
    bottom: 0,
    right: 0,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  }));

  return () => {
    Object.defineProperty(
      HTMLElement.prototype,
      'offsetHeight',
      originalOffsetHeight!,
    );
    Object.defineProperty(
      HTMLElement.prototype,
      'offsetWidth',
      originalOffsetWidth!,
    );
    Element.prototype.getBoundingClientRect = originalGetBoundingClientRect;
  };
};
