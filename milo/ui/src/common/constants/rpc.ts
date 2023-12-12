// Copyright 2020 The LUCI Authors.
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

import { RpcCode } from '@chopsui/prpc-client';

/**
 * A list of RPC Error code that MAY indicate the user lacks permission.
 */
export const POTENTIAL_PERM_ERROR_CODES = Object.freeze([
  // Some RPCs will return NOT_FOUND when user isn't able to access it.
  RpcCode.NOT_FOUND,
  RpcCode.PERMISSION_DENIED,
  RpcCode.UNAUTHENTICATED,
]);

/**
 * A list of RPC Error code that indicates the error is non-transient.
 */
export const NON_TRANSIENT_ERROR_CODES = Object.freeze([
  RpcCode.INVALID_ARGUMENT,
  RpcCode.PERMISSION_DENIED,
  RpcCode.UNAUTHENTICATED,
  RpcCode.UNIMPLEMENTED,
]);
