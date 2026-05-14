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

import { BinaryReader, BinaryWriter } from '@bufbuild/protobuf/wire';
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import {
  UseQueryOptions,
  UseQueryResult,
  useQuery,
} from '@tanstack/react-query';

import {
  TokenType,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';
import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';

// Interface mimicking ts-proto generated MessageFns
export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  create(base?: any): T;
}

export interface GrpcWebQueryArgs<Req, Res> {
  host: string;
  service: string;
  method: string;
  request: Req;
  requestMsg: MessageFns<Req>;
  responseMsg: MessageFns<Res>;
}

function createGrpcErrorFromResponse(
  status: number,
  resText: string,
): GrpcError {
  switch (status) {
    case 400:
      return new GrpcError(
        RpcCode.INVALID_ARGUMENT,
        resText || 'invalid argument',
      );
    case 401:
      return new GrpcError(
        RpcCode.UNAUTHENTICATED,
        resText || 'unauthenticated',
      );
    case 403:
      return new GrpcError(
        RpcCode.PERMISSION_DENIED,
        resText || 'permission denied',
      );
    case 404:
      return new GrpcError(RpcCode.NOT_FOUND, resText || 'not found');
    case 429:
      return new GrpcError(
        RpcCode.RESOURCE_EXHAUSTED,
        resText || 'resource exhausted',
      );
    case 502:
    case 503:
      return new GrpcError(
        RpcCode.UNAVAILABLE,
        resText || 'service unavailable',
      );
    default:
      return new GrpcError(RpcCode.INTERNAL, resText || 'internal error');
  }
}

export const useGrpcWebQuery = <Req, Res>(
  args: GrpcWebQueryArgs<Req, Res>,
  queryOptions?: WrapperQueryOptions<Res>,
): UseQueryResult<Res> => {
  const getAccessToken = useGetAuthToken(TokenType.Access);
  const { host, service, method, request, requestMsg, responseMsg } = args;

  const options: UseQueryOptions<Res> = {
    ...queryOptions,
    queryKey: ['grpc-web', host, service, method, request],
    queryFn: async (): Promise<Res> => {
      const accessToken = await getAccessToken();

      const reqMessage = requestMsg.create(request);
      const requestBinary = requestMsg.encode(reqMessage).finish();

      const url = `${host}/$rpc/${service}/${method}`;

      const response = await fetch(url, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${accessToken}`,
          'Content-Type': 'application/x-protobuf',
        },
        body: requestBinary as BodyInit,
      });

      if (!response.ok) {
        const resText = await response.text();
        throw createGrpcErrorFromResponse(response.status, resText);
      }

      const responseBuffer = await response.arrayBuffer();
      return responseMsg.decode(new Uint8Array(responseBuffer));
    },
  };

  return useQuery(options);
};
