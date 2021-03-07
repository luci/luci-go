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

import { PrpcClient } from '@chopsui/prpc-client';

export class CachedPrpcClient {
  private readonly cache = new Map<string, Promise<unknown>>();

  constructor(private readonly client: PrpcClient) {}

  /**
   * A wrapper of prpcClient.call.
   * When forceRefresh is not set to true and there was an identical call,
   * returned the cached response.
   */
  call(
    service: string,
    method: string,
    message: object,
    additionalHeaders?: {[key: string]: string},
    forceRefresh = false,
) {
    const key = `${service}-${method}-${JSON.stringify(message)}-${JSON.stringify(additionalHeaders || {})}`;
    const res = !forceRefresh && this.cache.get(key)
      || this.client.call(service, method, message, additionalHeaders);
    this.cache.set(key, res);
    return res;
  }
}
