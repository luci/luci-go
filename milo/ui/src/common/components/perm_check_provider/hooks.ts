// Copyright 2024 The LUCI Authors.
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

import { useQuery } from '@tanstack/react-query';
import { useContext } from 'react';

import { ClientCtx } from './context';

/**
 * Checks whether the user has permission `perm` in realm `realm`.
 *
 * All permission checks in the same rendering cycle are batched into a single
 * RPC request and the results are cached by react-query.
 *
 * When either `perm` or `realm` is not specified, the query will not be sent.
 *
 * N.B. Only permissions registered [here][1] can be checked.
 *
 * [1]: https://source.corp.google.com/h/github/googlesource/codesearch/+/master:luci/luci-go/milo/rpc/batch_check_permissions.go;l=26;drc=e952557609eade6037c66866ebc00c506d8800a8
 */
export function usePermCheck(
  realm?: string | null,
  perm?: string | null,
): [allowed: boolean, isLoading: boolean] {
  const client = useContext(ClientCtx);
  if (!client) {
    throw new Error('usePermCheck can only be used in a PermCheckProvider');
  }

  const { data, isError, error, isLoading } = useQuery({
    ...client.BatchCheckPermissions.query({
      realm: realm!,
      permissions: [perm!],
    }),
    select(data) {
      return data.results[perm!];
    },
    enabled: Boolean(realm) && Boolean(perm),
  });
  if (isError) {
    throw error;
  }
  return [data ?? false, isLoading];
}
