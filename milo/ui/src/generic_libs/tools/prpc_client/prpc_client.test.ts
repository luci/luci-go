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

import { GrpcError, ProtocolError, RpcCode } from '@chopsui/prpc-client';

import { PrpcClient } from './prpc_client';

describe('PrpcClient', () => {
  it('without token', async () => {
    const mockedFetch = jest.fn(fetch).mockResolvedValue(
      new Response(')]}\'\n{"key":"response"}', {
        headers: {
          'X-Prpc-Grpc-Code': RpcCode.OK.toString(),
        },
      }),
    );
    const client = new PrpcClient({
      host: 'host.com',
      fetchImpl: mockedFetch,
      getAuthToken: () => '',
    });

    const res = await client.request('service', 'method', { key: 'request' });
    expect(mockedFetch).toHaveBeenCalledWith(
      'https://host.com/prpc/service/method',
      {
        body: JSON.stringify({ key: 'request' }),
        credentials: 'omit',
        headers: {
          accept: 'application/json',
          'content-type': 'application/json',
        },
        method: 'POST',
      },
    );
    expect(res).toEqual({ key: 'response' });
  });

  it('with token', async () => {
    const mockedFetch = jest.fn(fetch).mockResolvedValue(
      new Response(')]}\'\n{"key":"response"}', {
        headers: {
          'X-Prpc-Grpc-Code': RpcCode.OK.toString(),
        },
      }),
    );
    const client = new PrpcClient({
      host: 'host.com',
      fetchImpl: mockedFetch,
      getAuthToken: () => 'auth token',
    });

    const res = await client.request('service', 'method', { key: 'request' });
    expect(mockedFetch).toHaveBeenCalledWith(
      'https://host.com/prpc/service/method',
      {
        body: JSON.stringify({ key: 'request' }),
        credentials: 'omit',
        headers: {
          accept: 'application/json',
          'content-type': 'application/json',
          authorization: 'Bearer auth token',
        },
        method: 'POST',
      },
    );
    expect(res).toEqual({ key: 'response' });
  });

  it('RPC error', async () => {
    const mockedFetch = jest.fn(fetch).mockResolvedValue(
      new Response('not found', {
        headers: {
          'X-Prpc-Grpc-Code': RpcCode.NOT_FOUND.toString(),
        },
      }),
    );
    const client = new PrpcClient({
      host: 'host.com',
      fetchImpl: mockedFetch,
    });

    const call = client.request('service', 'method', { key: 'request' });
    await expect(call).rejects.toEqual(expect.any(GrpcError));
    const err = (await call.catch((e) => e)) as GrpcError;
    expect(err.code).toEqual(RpcCode.NOT_FOUND);
    expect(err.description).toEqual('not found');
  });

  it('protocol error', async () => {
    const mockedFetch = jest
      .fn(fetch)
      .mockResolvedValue(new Response(')]}\'\n{"key":"response"}'));
    const client = new PrpcClient({
      host: 'host.com',
      fetchImpl: mockedFetch,
    });

    const call = client.request('service', 'method', { key: 'request' });
    await expect(call).rejects.toEqual(expect.any(ProtocolError));
    const err = (await call.catch((e) => e)) as ProtocolError;
    expect(err.httpStatus).toEqual(200);
    expect(err.message).toContain('no X-Prpc-Grpc-Code response header');
  });

  it('invalid RPC code', async () => {
    const mockedFetch = jest.fn(fetch).mockResolvedValue(
      new Response(')]}\'\n{"key":"response"}', {
        status: 400,
        headers: {
          'X-Prpc-Grpc-Code': 'not a string',
        },
      }),
    );
    const client = new PrpcClient({
      host: 'host.com',
      fetchImpl: mockedFetch,
    });

    const call = client.request('service', 'method', { key: 'request' });
    await expect(call).rejects.toEqual(expect.any(ProtocolError));
    const err = (await call.catch((e) => e)) as ProtocolError;
    expect(err.httpStatus).toEqual(400);
    expect(err.message).toContain('Invalid X-Prpc-Grpc-Code response header');
  });
});
