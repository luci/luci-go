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

import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import {
  LogRequest,
  LogResponse,
} from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';
import {
  MiloInternalClientImpl,
  ProxyGitilesLogRequest,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import {
  CrrevClientImpl,
  NumberingRequest,
  NumberingResponse,
} from '@/proto/infra/appengine/cr-rev/frontend/api/v1/service.pb';

import {
  FusedGitilesClientImpl,
  ExtendedLogRequest,
} from './fused_gitiles_client';
import { RestGitilesClientImpl } from './rest_gitiles_client';

describe('FusedGitilesClientImpl', () => {
  describe('Log', () => {
    let gitilesLogSpy: jest.SpyInstance<
      Promise<LogResponse>,
      [request: LogRequest]
    >;
    let proxyGitilesLogSpy: jest.SpyInstance<
      Promise<LogResponse>,
      [request: ProxyGitilesLogRequest]
    >;

    beforeEach(() => {
      jest.useFakeTimers();
      gitilesLogSpy = jest.spyOn(RestGitilesClientImpl.prototype, 'Log');
      proxyGitilesLogSpy = jest.spyOn(
        MiloInternalClientImpl.prototype,
        'ProxyGitilesLog',
      );
    });

    afterEach(() => {
      jest.useRealTimers();
      jest.resetAllMocks();
    });

    it('should work with CORS-enabled host', async () => {
      const logRes = LogResponse.fromPartial({
        log: [{ author: { email: 'email@email.com', name: 'name' } }],
      });
      gitilesLogSpy.mockResolvedValueOnce(logRes);
      const client = new FusedGitilesClientImpl(
        new PrpcClient({ host: 'chromium.googlesource.com' }),
        { crRevHost: 'cr-rev.host' },
      );
      const res = await client.Log(
        LogRequest.fromPartial({
          project: 'the_project',
          committish: 'hash_for_1234',
          pageSize: 10,
        }),
      );
      expect(res).toEqual(logRes);
      expect(proxyGitilesLogSpy).not.toHaveBeenCalled();
      expect(gitilesLogSpy).toHaveBeenCalledTimes(1);
      expect(gitilesLogSpy).toHaveBeenNthCalledWith(
        1,
        LogRequest.fromPartial({
          committish: 'hash_for_1234',
          pageSize: 10,
          project: 'the_project',
        }),
      );
    });

    it('should work with non-CORS-enabled host', async () => {
      const logRes = LogResponse.fromPartial({
        log: [{ author: { email: 'email@email.com', name: 'name' } }],
      });
      proxyGitilesLogSpy.mockResolvedValueOnce(logRes);
      const client = new FusedGitilesClientImpl(
        new PrpcClient({ host: 'other.googlesource.com' }),
        { crRevHost: 'cr-rev.host' },
      );
      const res = await client.Log(
        LogRequest.fromPartial({
          project: 'the_project',
          committish: 'hash_for_1234',
          pageSize: 10,
        }),
      );
      expect(res).toEqual(logRes);
      expect(gitilesLogSpy).not.toHaveBeenCalled();
      expect(proxyGitilesLogSpy).toHaveBeenCalledTimes(1);
      expect(proxyGitilesLogSpy).toHaveBeenNthCalledWith(
        1,
        ProxyGitilesLogRequest.fromPartial({
          host: 'other.googlesource.com',
          request: {
            committish: 'hash_for_1234',
            pageSize: 10,
            project: 'the_project',
          },
        }),
      );
    });
  });

  describe('ExtendedLog', () => {
    let numberingSpy: jest.SpyInstance<
      Promise<NumberingResponse>,
      [request: NumberingRequest]
    >;
    let proxyGitilesLogSpy: jest.SpyInstance<
      Promise<LogResponse>,
      [request: ProxyGitilesLogRequest]
    >;

    beforeEach(() => {
      jest.useFakeTimers();
      numberingSpy = jest.spyOn(CrrevClientImpl.prototype, 'Numbering');
      proxyGitilesLogSpy = jest.spyOn(
        MiloInternalClientImpl.prototype,
        'ProxyGitilesLog',
      );
    });

    afterEach(() => {
      jest.useRealTimers();
      jest.resetAllMocks();
    });

    it('should work with regular query', async () => {
      const logRes = LogResponse.fromPartial({
        log: [{ author: { email: 'email@email.com', name: 'name' } }],
      });
      proxyGitilesLogSpy.mockResolvedValueOnce(logRes);
      const client = new FusedGitilesClientImpl(
        new PrpcClient({ host: 'gitiles_host.googlesource.com' }),
        { crRevHost: 'cr-rev.host' },
      );
      const res = await client.ExtendedLog(
        ExtendedLogRequest.fromPartial({
          project: 'the_project',
          committish: 'hash_for_1234',
          pageSize: 10,
        }),
      );
      expect(res).toEqual(logRes);
      expect(numberingSpy).not.toHaveBeenCalled();
      expect(proxyGitilesLogSpy).toHaveBeenCalledTimes(1);
      expect(proxyGitilesLogSpy).toHaveBeenNthCalledWith(
        1,
        ProxyGitilesLogRequest.fromPartial({
          host: 'gitiles_host.googlesource.com',
          request: {
            committish: 'hash_for_1234',
            pageSize: 10,
            project: 'the_project',
          },
        }),
      );
    });

    it('should work with commit position query', async () => {
      numberingSpy.mockResolvedValueOnce(
        NumberingResponse.fromPartial({ gitHash: 'hash_for_1234' }),
      );
      const logRes = LogResponse.fromPartial({
        log: [{ author: { email: 'email@email.com', name: 'name' } }],
      });
      proxyGitilesLogSpy.mockResolvedValueOnce(logRes);
      const client = new FusedGitilesClientImpl(
        new PrpcClient({ host: 'gitiles_host.googlesource.com' }),
        { crRevHost: 'cr-rev.host' },
      );
      const res = await client.ExtendedLog(
        ExtendedLogRequest.fromPartial({
          project: 'the_project',
          ref: 'a_branch',
          position: '1234',
          pageSize: 10,
        }),
      );
      expect(res).toEqual(logRes);
      expect(numberingSpy).toHaveBeenCalledTimes(1);
      expect(numberingSpy).toHaveBeenNthCalledWith(
        1,
        NumberingRequest.fromPartial({
          host: 'gitiles_host',
          positionNumber: '1234',
          positionRef: 'a_branch',
          repository: 'the_project',
        }),
      );
      expect(proxyGitilesLogSpy).toHaveBeenCalledTimes(1);
      expect(proxyGitilesLogSpy).toHaveBeenNthCalledWith(
        1,
        ProxyGitilesLogRequest.fromPartial({
          host: 'gitiles_host.googlesource.com',
          request: {
            committish: 'hash_for_1234',
            pageSize: 10,
            project: 'the_project',
          },
        }),
      );
    });
  });
});
