// Copyright 2026 The LUCI Authors.
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

import { isScriptError, registerErrorHandlers } from './register_handlers';

describe('isScriptError', () => {
  it('should return true for undefined or empty string', () => {
    expect(isScriptError(undefined)).toBe(true);
    expect(isScriptError('')).toBe(true);
    expect(isScriptError('   ')).toBe(true);
  });

  it('should return true for standard "Script error."', () => {
    expect(isScriptError('Script error.')).toBe(true);
    expect(isScriptError('Script error')).toBe(true);
    expect(isScriptError('script error.')).toBe(true);
    expect(isScriptError('SCRIPT ERROR.')).toBe(true);
  });

  it('should return true for browser-prefixed Script errors', () => {
    expect(isScriptError('Javascript error: Script error.')).toBe(true);
    expect(isScriptError('javascript error: Script error')).toBe(true);
    expect(isScriptError('JavaScript Error: Script error.')).toBe(true);
  });

  it('should return false for genuine error messages', () => {
    expect(isScriptError('Uncaught TypeError: Cannot read property')).toBe(
      false,
    );
    expect(isScriptError('SyntaxError: Unexpected token <')).toBe(false);
    expect(isScriptError('Failed to fetch script')).toBe(false);
  });
});

describe('registerErrorHandlers', () => {
  let mockReporter: jest.Mock;
  let errorListeners: ((event: ErrorEvent) => void)[];
  let unhandledRejectionListeners: ((event: PromiseRejectionEvent) => void)[];

  beforeEach(() => {
    mockReporter = jest.fn();
    errorListeners = [];
    unhandledRejectionListeners = [];

    jest
      .spyOn(window, 'addEventListener')
      .mockImplementation((type, listener) => {
        if (type === 'error') {
          errorListeners.push(listener as (event: ErrorEvent) => void);
        } else if (type === 'unhandledrejection') {
          unhandledRejectionListeners.push(
            listener as (event: PromiseRejectionEvent) => void,
          );
        }
      });

    registerErrorHandlers(mockReporter);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('window error event', () => {
    it('should report standard Error instance', () => {
      const error = new Error('Regular error');
      const event = new ErrorEvent('error', { error });

      errorListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter).toHaveBeenCalledWith(error);
    });

    it('should ignore Error instance with "Script error." message', () => {
      const error = new Error('Script error.');
      const event = new ErrorEvent('error', { error });

      errorListeners.forEach((listener) => listener(event));

      expect(mockReporter).not.toHaveBeenCalled();
    });

    it('should report non-Error thrown objects', () => {
      const event = {
        error: { customCode: 123 },
      } as unknown as ErrorEvent;

      errorListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter.mock.calls[0][0].message).toBe(
        'Non-error value thrown: {"customCode":123}',
      );
    });

    it('should ignore masked "Script error." when error object is missing', () => {
      const event = {
        message: 'Script error.',
        filename: '',
        lineno: 0,
        colno: 0,
        error: null,
      } as unknown as ErrorEvent;

      errorListeners.forEach((listener) => listener(event));

      expect(mockReporter).not.toHaveBeenCalled();
    });

    it('should report legacy error when error object is missing but message is valid', () => {
      const event = {
        message: 'Uncaught ReferenceError: foo is not defined',
        filename: 'https://ci.chromium.org/main.js',
        lineno: 42,
        colno: 10,
        error: null,
      } as unknown as ErrorEvent;

      errorListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter.mock.calls[0][0].message).toBe(
        'Uncaught ReferenceError: foo is not defined at https://ci.chromium.org/main.js:42:10',
      );
    });
  });

  describe('unhandledrejection event', () => {
    it('should report Promise rejection with Error reason', () => {
      const reason = new Error('Rejection error');
      const event = { reason } as PromiseRejectionEvent;

      unhandledRejectionListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter).toHaveBeenCalledWith(reason);
    });

    it('should ignore Promise rejection with "Script error." Error', () => {
      const reason = new Error('Script error.');
      const event = { reason } as PromiseRejectionEvent;

      unhandledRejectionListeners.forEach((listener) => listener(event));

      expect(mockReporter).not.toHaveBeenCalled();
    });

    it('should ignore Promise rejection with "Script error." string', () => {
      const event = { reason: 'Script error.' } as PromiseRejectionEvent;

      unhandledRejectionListeners.forEach((listener) => listener(event));

      expect(mockReporter).not.toHaveBeenCalled();
    });

    it('should report non-Error rejection reason', () => {
      const event = { reason: { status: 500 } } as PromiseRejectionEvent;

      unhandledRejectionListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter.mock.calls[0][0].message).toBe(
        'Unhandled promise rejection: {"status":500}',
      );
    });

    it('should report undefined or null rejection reason', () => {
      const event = { reason: undefined } as unknown as PromiseRejectionEvent;

      unhandledRejectionListeners.forEach((listener) => listener(event));

      expect(mockReporter).toHaveBeenCalledTimes(1);
      expect(mockReporter.mock.calls[0][0].message).toBe(
        'Unhandled promise rejection: undefined',
      );
    });
  });
});
