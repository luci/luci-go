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
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const responseBuffer = await response.arrayBuffer();
      return responseMsg.decode(new Uint8Array(responseBuffer));
    },
  };

  return useQuery(options);
};
