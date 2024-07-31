// Copyright 2023 The LUCI Authors.
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
   * pRPC server host.
   */
  readonly host: string;
  /**
   * Auth token to use in RPC. Defaults to `() => ''`.
   */
  readonly getAuthToken?: () => string | Promise<string>;
  /**
   * Token type returned by `getAuthToken`. Defaults to `Bearer`.
   */
  readonly tokenType?: string;
  /**
   * Additional headers to be passed to the RPC call. This can be used to pass
   * [gRPC metadata](https://grpc.io/docs/guides/metadata/#headers) to the
   * server.
   *
   * Note that this does not override the headers set by `PrpcClient`, including
   *  * accept
   *  * content-type
   *  * authorization
   */
  readonly additionalHeaders?: HeadersInit;
  /**
   * If true, use HTTP instead of HTTPS. Defaults to `false`.
   */
  readonly insecure?: boolean;
  /**
   * If supplied, use this function instead of fetch.
   */
  readonly fetchImpl?: typeof fetch;
}

const NON_OVERRIDABLE_HEADERS = Object.freeze([
  'accept',
  'content-type',
  'authorization',
]);

/**
 * Class for interacting with a pRPC API with a binary protocol.
 * Protocol: https://godoc.org/go.chromium.org/luci/grpc/prpc
 */
export class PrpcClient {
  readonly host: string;
  readonly getAuthToken: () => string | Promise<string>;
  readonly tokenType: string;
  readonly additionalHeaders: Readonly<Record<string, string>>;
  readonly insecure: boolean;
  readonly fetchImpl: typeof fetch;

  constructor(options: PrpcClientOptions) {
    const headers = new Headers(options?.additionalHeaders);
    for (const key of headers.keys()) {
      if (NON_OVERRIDABLE_HEADERS.includes(key)) {
        throw new Error(`\`${key}\` cannot be specified as additionalHeaders`);
      }
    }

    this.host = options.host;
    this.getAuthToken = options.getAuthToken || (() => '');
    this.tokenType = options.tokenType || 'Bearer';
    this.additionalHeaders = Object.fromEntries(headers.entries());
    this.insecure = options.insecure || false;
    this.fetchImpl = options.fetchImpl || self.fetch.bind(self);
  }

  /**
   * Send an RPC request.
   * @param service Full service name, including package name.
   * @param method  Service method name.
   * @param data The protobuf message to send in canonical JSON format.
   * @throws {ProtocolError} when an error happens at the pRPC protocol
   * (HTTP) level.
   * @throws {GrpcError} when the response returns a non-OK gRPC status.
   * @return a promise resolving the response message in binary format.
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
        ...this.additionalHeaders,
        accept: 'application/json',
        'content-type': 'application/json',
        ...(token && { authorization: `${this.tokenType} ${token}` }),
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
      response.headers.get('X-Prpc-Grpc-Code')!,
      10,
    );
    if (Number.isNaN(rpcCode)) {
      throw new ProtocolError(
        response.status,
        `Invalid X-Prpc-Grpc-Code response header`,
      );
    }

    const text = await response.text();

    if (rpcCode !== RpcCode.OK) {
      throw new GrpcError(rpcCode, text);
    }

    // Strips out the XSSI prefix.
    return JSON.parse(text.slice(")]}'\n".length));
  }
}
