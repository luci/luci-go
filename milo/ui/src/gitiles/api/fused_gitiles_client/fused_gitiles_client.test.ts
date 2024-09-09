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
  QueryCommitHashRequest,
  QueryCommitHashResponse,
  SourceIndexClientImpl,
} from '@/proto/go.chromium.org/luci/source_index/proto/v1/source_index.pb';

import { RestGitilesClientImpl } from '../rest_gitiles_client';

import {
  FusedGitilesClientImpl,
  ExtendedLogRequest,
} from './fused_gitiles_client';

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
        { sourceIndexHost: 'source-index.host' },
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
        { sourceIndexHost: 'source-index.host' },
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
    let queryCommitHashSpy: jest.SpyInstance<
      Promise<QueryCommitHashResponse>,
      [request: QueryCommitHashRequest]
    >;
    let proxyGitilesLogSpy: jest.SpyInstance<
      Promise<LogResponse>,
      [request: ProxyGitilesLogRequest]
    >;

    beforeEach(() => {
      jest.useFakeTimers();
      queryCommitHashSpy = jest.spyOn(
        SourceIndexClientImpl.prototype,
        'QueryCommitHash',
      );
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
        { sourceIndexHost: 'source-index.host' },
      );
      const res = await client.ExtendedLog(
        ExtendedLogRequest.fromPartial({
          project: 'the_project',
          committish: 'hash_for_1234',
          pageSize: 10,
        }),
      );
      expect(res).toEqual(logRes);
      expect(queryCommitHashSpy).not.toHaveBeenCalled();
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
      queryCommitHashSpy.mockResolvedValueOnce(
        QueryCommitHashResponse.fromPartial({ hash: 'hash_for_1234' }),
      );
      const logRes = LogResponse.fromPartial({
        log: [{ author: { email: 'email@email.com', name: 'name' } }],
      });
      proxyGitilesLogSpy.mockResolvedValueOnce(logRes);
      const client = new FusedGitilesClientImpl(
        new PrpcClient({ host: 'gitiles_host.googlesource.com' }),
        { sourceIndexHost: 'source-index.host' },
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
      expect(queryCommitHashSpy).toHaveBeenCalledTimes(1);
      expect(queryCommitHashSpy).toHaveBeenNthCalledWith(
        1,
        QueryCommitHashRequest.fromPartial({
          host: 'gitiles_host.googlesource.com',
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
            committish: 'hash_for_1234~0',
            pageSize: 10,
            project: 'the_project',
          },
        }),
      );
    });
  });
});
