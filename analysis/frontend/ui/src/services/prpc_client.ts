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

// Import the error and RPC code types from '@chopsui/prpc-client' so we can
// handle the errors from the binary client and the original client the same
// way.
//
// TODO(crbug/1504937): drop the '@chopsui/prpc-client' and declare our own
// error and RPC code types once all other usage of prpc-client is migrated to
// the binary client.
import { GrpcError, ProtocolError, RpcCode } from '@chopsui/prpc-client';

export interface PrpcClientOptions {
  /**
   * pRPC server host, defaults to current document host.
   */
  readonly host?: string;
  /**
   * Auth token to use in RPC. Defaults to `() => ''`.
   */
  readonly getAuthToken?: () => string | Promise<string>;
  /**
   * If true, use HTTP instead of HTTPS. Defaults to `false`.
   */
  readonly insecure?: boolean;
  /**
   * If supplied, use this function instead of fetch.
   */
  readonly fetchImpl?: typeof fetch;
}

/**
 * Class for interacting with a pRPC API with a JSON protocol.
 * Protocol: https://godoc.org/go.chromium.org/luci/grpc/prpc
 */
export class PrpcClient {
  readonly host: string;
  readonly getAuthToken: () => string | Promise<string>;
  readonly insecure: boolean;
  readonly fetchImpl: typeof fetch;

  constructor(options?: PrpcClientOptions) {
    this.host = options?.host || self.location.host;
    this.getAuthToken = options?.getAuthToken || (() => '');
    this.insecure = options?.insecure || false;
    this.fetchImpl = options?.fetchImpl || self.fetch.bind(self);
  }

  /**
   * Send an RPC request.
   * @param {string} service Full service name, including package name.
   * @param {string} method Service method name.
   * @param {unknown} data The protobuf message object to send.
   * @throws {ProtocolError} when an error happens at the pRPC protocol
   * (HTTP) level.
   * @throws {GrpcError} when the response returns a non-OK gRPC status.
   * @return {Promise<unknown>} a promise resolving the response message object.
   */
  async request(
      service: string,
      method: string,
      data: unknown,
  ): Promise<unknown> {
    const protocol = this.insecure ? 'http:' : 'https:';
    const url = `${protocol}//${this.host}/prpc/${service}/${method}`;

    const token = await this.getAuthToken();
    const response = await this.fetchImpl(url, {
      method: 'POST',
      credentials: 'omit',
      headers: {
        'accept': 'application/json',
        'content-type': 'application/json',
        ...(token && { authorization: `Bearer ${token}` }),
      },
      body: JSON.stringify(data),
    });

    if (!response.headers.has('X-Prpc-Grpc-Code')) {
      throw new ProtocolError(
          response.status,
          'Invalid response: no X-Prpc-Grpc-Code response header',
      );
    }

    const rpcCode = Number.parseInt(
        response.headers.get('X-Prpc-Grpc-Code') || '',
        10,
    );
    if (Number.isNaN(rpcCode)) {
      throw new ProtocolError(
          response.status,
          'Invalid X-Prpc-Grpc-Code response header',
      );
    }

    const text = await response.text();

    if (rpcCode !== RpcCode.OK) {
      throw new GrpcError(rpcCode, text);
    }

    // Strips out the XSSI prefix.
    return JSON.parse(text.slice(')]}\'\n'.length));
  }
}
