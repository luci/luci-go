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
  if (error instanceof GrpcError) {
    if (error.code === 7) {
      return `You don't have permission to ${operation}`;
    }

    return error.description;
  }
  // TODO: b/401486024 - Handle more error types.
  return 'Unknown error';
};
