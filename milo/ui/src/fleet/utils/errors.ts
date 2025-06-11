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

/**
 * Retrieves a human readable description of an error.
 * @param error Error to handle.
 * @param operation Operation the user was performing.
 * @returns String with formatted error message.
 */
export const getErrorMessage = (error: unknown, operation: string): string => {
  // error codes: https://grpc.io/docs/guides/status-codes/
  if (error instanceof GrpcError) {
    switch (error.code) {
      case 1: // CANCELLED
        return `The operation for ${operation} was cancelled.`;
      case 3: // INVALID_ARGUMENT
        return `The information provided for ${operation} is invalid. Please check your input and try again.`;
      case 4: // DEADLINE_EXCEEDED
        return `The operation for ${operation} took too long to complete. Please try again.`;
      case 5: // NOT_FOUND
        return `The requested item for ${operation} could not be found.`;
      case 6: // ALREADY_EXISTS
        return `An item with the same name already exists. Please choose a different name for ${operation}.`;
      case 7: // PERMISSION_DENIED
        return `You don't have permission to ${operation}.`;
      case 8: // RESOURCE_EXHAUSTED
        return `We've reached our capacity for ${operation}. Please try again later.`;
      case 10: // ABORTED
        return `The operation for ${operation} was cancelled due to a conflict. Please try again.`;
      case 12: // UNIMPLEMENTED
        return `The operation ${operation} is not implemented or not supported/enabled in this service.`;
      case 14: // UNAVAILABLE
        return `The service for ${operation} is currently unavailable. Please try again in a few moments.`;
      case 16: // UNAUTHENTICATED
        return `You are not authenticated to perform the operation ${operation}. Please log in and try again.`;
      default:
        return `An unexpected error occurred during ${operation}. ${error.description}.`;
    }
  }

  if (error instanceof Error) {
    if (error instanceof TypeError) {
      return `There was a problem processing the data for ${operation}.`;
    }
    if (error instanceof RangeError) {
      return `A value provided for ${operation} is outside of the acceptable range.`;
    }

    return `An unexpected error occurred: ${error.message || 'Something went wrong.'}`;
  }

  return 'An unknown error occurred. Please try again.';
};
