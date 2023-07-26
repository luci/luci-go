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

import { GrpcError, PrpcClient, PrpcClientOptions } from '@chopsui/prpc-client';

const DEFAULT_ON_ERROR_FN = (e: GrpcError) => {
  throw e;
};

/**
 * Extends the PrpcClient to support updating accessToken and wrapping errors.
 */
export class PrpcClientExt {
  private client: PrpcClient;

  constructor(
    opts: PrpcClientOptions,
    private readonly getAccessToken: () => Promise<string> | string,
    private readonly onError = DEFAULT_ON_ERROR_FN
  ) {
    this.client = new PrpcClient({
      ...opts,
      accessToken: undefined,
    });
  }

  async call(
    service: string,
    method: string,
    message: object,
    additionalHeaders: { [key: string]: string } = {}
  ) {
    const accessToken = await this.getAccessToken();
    if (accessToken) {
      additionalHeaders = {
        Authorization: 'Bearer ' + accessToken,
        ...additionalHeaders,
      };
    }
    return this.client
      .call(service, method, message, additionalHeaders)
      .catch((e) => this.onError(e));
  }
}
