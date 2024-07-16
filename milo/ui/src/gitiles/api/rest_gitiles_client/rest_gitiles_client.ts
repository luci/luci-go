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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';

import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import {
  ArchiveRequest,
  ArchiveResponse,
  DownloadDiffRequest,
  DownloadDiffResponse,
  DownloadFileRequest,
  DownloadFileResponse,
  Gitiles,
  ListFilesRequest,
  ListFilesResponse,
  LogRequest,
  LogResponse,
  ProjectsRequest,
  ProjectsResponse,
  RefsRequest,
  RefsResponse,
} from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';

import { RestLogResponse } from './types';

const GITILES_HOST_RE = /^[a-z0-9]([a-z0-9-]*[a-z0-9])*.googlesource.com$/;

export const GitilesServiceName = 'gitiles.Gitiles';
export class RestGitilesClientImpl implements Gitiles {
  static readonly DEFAULT_SERVICE = GitilesServiceName;
  readonly service: string;
  constructor(
    private readonly rpc: PrpcClient,
    opts?: { service?: string },
  ) {
    if (this.rpc.insecure) {
      throw new Error('gitiles client must be a secure client');
    }
    // Gitiles hosts are often specified by the untrusted input (e.g. build
    // data). Ensure we only send access token to valid gitiles hosts.
    if (!GITILES_HOST_RE.test(this.rpc.host)) {
      throw new Error(`${this.rpc.host} is not a valid gitiles host`);
    }

    this.service = opts?.service || GitilesServiceName;
    this.Log = this.Log.bind(this);
    this.Refs = this.Refs.bind(this);
    this.Archive = this.Archive.bind(this);
    this.DownloadFile = this.DownloadFile.bind(this);
    this.DownloadDiff = this.DownloadDiff.bind(this);
    this.Projects = this.Projects.bind(this);
    this.ListFiles = this.ListFiles.bind(this);
  }

  /**
   * Sends a request to the gitiles host, handles the error and parse the
   * response.
   */
  private async rawRequest<Res>(
    project: string,
    path: string,
    params?: URLSearchParams,
  ): Promise<Res> {
    params = new URLSearchParams(params);
    params.set('format', 'JSON');

    const authToken = await this.rpc.getAuthToken();
    if (authToken) {
      // OAuth token needs to be passed via URL instead of via the
      // `Authorization` header, because
      //  * gitiles does not allow Authorization header in CORS request, and
      //  * see also http://go/gob/users/rest-api#access-and-authentication.
      //
      // `/a` can be added to the path prefix when sending an authorized
      // request. But it's not required when OAuth authentication is used.
      params.set('access_token', authToken);
    }
    const url = `https://${this.rpc.host}/${encodeURIComponent(project)}/${path}?${params}`;
    const req = await this.rpc.fetchImpl(url);
    const res = await req.text();
    switch (req.status) {
      case 200:
        return JSON.parse(res.slice(")]}'\n".length)) as Res;
      case 400:
        throw new GrpcError(RpcCode.INVALID_ARGUMENT, res);
      case 403:
        throw new GrpcError(RpcCode.PERMISSION_DENIED, 'permission denied');
      case 404:
        throw new GrpcError(RpcCode.NOT_FOUND, 'not found');
      case 429:
        throw new GrpcError(
          RpcCode.RESOURCE_EXHAUSTED,
          'insufficient Gitiles quota',
        );
      case 502:
        throw new GrpcError(RpcCode.UNAVAILABLE, 'bad gateway');
      case 503:
        throw new GrpcError(RpcCode.UNAVAILABLE, 'service unavailable');
      default:
        throw new GrpcError(
          RpcCode.INTERNAL,
          `unexpected HTTP ${req.status} from Gitiles`,
        );
    }
  }

  async Log(request: LogRequest): Promise<LogResponse> {
    const params = new URLSearchParams();
    if (request.pageSize > 0) {
      params.set('n', request.pageSize.toString());
    }
    if (request.treeDiff) {
      params.set('name-status', '1');
    }
    if (request.pageToken) {
      params.set('s', request.pageSize.toString());
    }
    const ref = request.excludeAncestorsOf
      ? `${request.excludeAncestorsOf}..${request.committish}`
      : request.committish;
    const path = `+log/${encodeURIComponent(ref)}${request.path ? `/${request.path}` : ''}`;

    const resPayload = await this.rawRequest<RestLogResponse>(
      request.project,
      path,
      params,
    );
    return RestLogResponse.toProto(resPayload);
  }

  Refs(_request: RefsRequest): Promise<RefsResponse> {
    throw new Error('Method not implemented.');
  }
  Archive(_request: ArchiveRequest): Promise<ArchiveResponse> {
    throw new Error('Method not implemented.');
  }
  DownloadFile(_request: DownloadFileRequest): Promise<DownloadFileResponse> {
    throw new Error('Method not implemented.');
  }
  DownloadDiff(_request: DownloadDiffRequest): Promise<DownloadDiffResponse> {
    throw new Error('Method not implemented.');
  }
  Projects(_request: ProjectsRequest): Promise<ProjectsResponse> {
    throw new Error('Method not implemented.');
  }
  ListFiles(_request: ListFilesRequest): Promise<ListFilesResponse> {
    throw new Error('Method not implemented.');
  }
}
