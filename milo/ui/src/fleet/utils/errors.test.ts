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

import { GrpcError } from '@chopsui/prpc-client';

import { getErrorMessage } from './errors';

describe('getErrorMessage', () => {
  const mockOperation = 'Create new user: orange tabby';

  const createMockGrpcError = (code: number, description?: string) => {
    return new GrpcError(code, description || '');
  };

  describe('GrpcError', () => {
    it('returns correct message for CANCELLED (code 1)', () => {
      const error = createMockGrpcError(1);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The operation for ${mockOperation} was cancelled.`,
      );
    });

    it('returns correct message for INVALID_ARGUMENT (code 3)', () => {
      const error = createMockGrpcError(3);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The information provided for ${mockOperation} is invalid. Please check your input and try again.`,
      );
    });

    it('returns correct message for DEADLINE_EXCEEDED (code 4)', () => {
      const error = createMockGrpcError(4);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The operation for ${mockOperation} took too long to complete. Please try again.`,
      );
    });

    it('returns correct message for NOT_FOUND (code 5)', () => {
      const error = createMockGrpcError(5);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The requested item for ${mockOperation} could not be found.`,
      );
    });

    it('returns correct message for ALREADY_EXISTS (code 6)', () => {
      const error = createMockGrpcError(6);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `An item with the same name already exists. Please choose a different name for ${mockOperation}.`,
      );
    });

    it('returns correct message for PERMISSION_DENIED (code 7)', () => {
      const error = createMockGrpcError(7);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `You don't have permission to ${mockOperation}.`,
      );
    });

    it('returns correct message for RESOURCE_EXHAUSTED (code 8)', () => {
      const error = createMockGrpcError(8);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `We've reached our capacity for ${mockOperation}. Please try again later.`,
      );
    });

    it('returns correct message for ABORTED (code 10)', () => {
      const error = createMockGrpcError(10);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The operation for ${mockOperation} was cancelled due to a conflict. Please try again.`,
      );
    });

    it('returns correct message for UNIMPLEMENTED (code 12)', () => {
      const error = createMockGrpcError(12);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The operation ${mockOperation} is not implemented or not supported/enabled in this service.`,
      );
    });

    it('returns correct message for UNAVAILABLE (code 14)', () => {
      const error = createMockGrpcError(14);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `The service for ${mockOperation} is currently unavailable. Please try again in a few moments.`,
      );
    });

    it('returns correct message for UNAUTHENTICATED (code 16)', () => {
      const error = createMockGrpcError(16);
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `You are not authenticated to perform the operation ${mockOperation}. Please log in and try again.`,
      );
    });

    it('returns generic message for an unhandled GrpcError code', () => {
      const unhandledCode = 404;
      const error = createMockGrpcError(
        unhandledCode,
        'Some specific unhandled description',
      );
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `An unexpected error occurred during ${mockOperation}. ${error.description}.`,
      );
    });
  });

  describe('returns correct message for standard Error types', () => {
    it('returns specific message for TypeError', () => {
      const error = new TypeError('Cannot read property of undefined');
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `There was a problem processing the data for ${mockOperation}.`,
      );
    });

    it('returns specific message for RangeError', () => {
      const error = new RangeError('Index out of bounds');
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `A value provided for ${mockOperation} is outside of the acceptable range.`,
      );
    });

    it('returns generic message for a regular Error', () => {
      const error = new Error('Something went wrong on the client side.');
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `An unexpected error occurred: Something went wrong on the client side.`,
      );
    });

    it('returns a fallback message for a generic Error with no message', () => {
      const error = new Error(); // Create an error without a specific message
      expect(getErrorMessage(error, mockOperation)).toEqual(
        `An unexpected error occurred: Something went wrong.`,
      );
    });
  });

  describe('Unknown error', () => {
    it('returns a generic message for an unknown error', () => {
      const error = 'Not an error object';
      expect(getErrorMessage(error, mockOperation)).toEqual(
        'An unknown error occurred. Please try again.',
      );
    });
    it('returns unknown error message for null', () => {
      const error = null;
      expect(getErrorMessage(error, mockOperation)).toEqual(
        'An unknown error occurred. Please try again.',
      );
    });

    it('returns unknown error message for undefined', () => {
      const error = undefined;
      expect(getErrorMessage(error, mockOperation)).toEqual(
        'An unknown error occurred. Please try again.',
      );
    });
  });
});
