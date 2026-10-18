// Copyright 2026 The LUCI Authors.
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

import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

import {
  buildPrototypeIndexHtml,
  buildPrototypeMetadata,
  formatUntrackedFileDiff,
  generatePrototypeSourceArtifacts,
  isExcludedFromPrototypePatch,
  parseCliArgs,
  slimUfsFleetClient,
  stripCrossOriginAttributes,
  stripProtoBinaryWireMethods,
  validateOutDir,
} from './build_prototype';

describe('build_prototype utilities', () => {
  it('renders prototype_index.html with inlined boot settings and no external CDN scripts', () => {
    const html = buildPrototypeIndexHtml('/ui/fleet/catalog');

    expect(html).toContain('window.__FCON_EASY_MOCK__ = true;');
    expect(html).toContain('<script id="fcon-inline-settings">');
    expect(html).toContain('var defaultRoute = "/ui/fleet/catalog";');
    expect(html).toContain('chromeos-swarming.appspot.com');
    expect(html).toContain('chrome-swarming.appspot.com');
    expect(html).toContain('src="/src/main.tsx"');
    expect(html).not.toContain('__FCON_DEFAULT_ROUTE__');
    expect(html).not.toContain('Content-Security-Policy');
    expect(html).not.toContain('googletagmanager.com');
    expect(html).not.toContain('apis.google.com');
  });

  it('strips crossorigin attributes emitted by vite:build-html', () => {
    const bundledHtml = [
      '<script type="module" crossorigin src="./assets/fleet.js"></script>',
      '<link rel="stylesheet" crossorigin="anonymous" href="./assets/fleet.css">',
    ].join('');

    const cleaned = stripCrossOriginAttributes(bundledHtml);

    expect(cleaned).not.toContain('crossorigin');
    expect(cleaned).toContain('src="./assets/fleet.js"');
  });

  it('strips unused protobuf binary encode and decode methods while preserving JSON methods', () => {
    const samplePb = [
      'export const Device: MessageFns<Device> = {',
      '  encode(message: Device, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {',
      '    return writer;',
      '  },',
      '  decode(input: BinaryReader | Uint8Array, length?: number): Device {',
      '    return { id: "" };',
      '  },',
      '  fromJSON(object: any): Device {',
      '    return { id: String(object.id ?? "") };',
      '  },',
      '  toJSON(message: Device): unknown {',
      '    return { id: message.id };',
      '  },',
      '};',
      'export interface MessageFns<T> {',
      '  encode(message: T, writer?: BinaryWriter): BinaryWriter;',
      '  decode(input: BinaryReader | Uint8Array, length?: number): T;',
      '  fromJSON(object: any): T;',
      '}',
    ].join('\n');

    const slimmed = stripProtoBinaryWireMethods(samplePb);

    expect(slimmed).toContain('encode(): any { return null; }');
    expect(slimmed).toContain('decode(): any { return null; }');
    expect(slimmed).toContain('fromJSON(object: any): Device');
    expect(slimmed).toContain('export interface MessageFns<T>');
    expect(slimmed).not.toContain('new BinaryWriter()');
  });

  it('preserves message boundaries when a message lacks fromJSON', () => {
    const multiPb = [
      'export const MsgWithoutFromJSON = {',
      '  encode(): void {},',
      '  decode(): void {},',
      '};',
      'export const MsgWithFromJSON = {',
      '  encode(): void {},',
      '  decode(): void {},',
      '  fromJSON(object: any) { return object; },',
      '};',
    ].join('\n');

    const slimmed = stripProtoBinaryWireMethods(multiPb);

    expect(slimmed).toContain('export const MsgWithoutFromJSON = {');
    expect(slimmed).toContain('export const MsgWithFromJSON = {');
  });

  it('replaces FleetClientImpl with a dynamic Proxy client forwarding UFS RPC methods', () => {
    const sampleFleetPb = [
      'export const FleetServiceName = "unifiedfleet.api.v1.rpc.Fleet";',
      'export class FleetClientImpl implements Fleet {',
      '  static readonly DEFAULT_SERVICE = FleetServiceName;',
      '  GetMachine() { return HeavyType.fromJSON({}); }',
      '}',
      'interface Rpc {',
      '  request(service: string, method: string, data: unknown): Promise<unknown>;',
      '}',
    ].join('\n');

    const slimmed = slimUfsFleetClient(sampleFleetPb);

    expect(slimmed).not.toContain('HeavyType.fromJSON');
    expect(slimmed).toContain('new Proxy(this');
    expect(slimmed).toContain('interface Rpc {');
  });

  it('formats untracked new files with 40-char blob SHAs and excludes builder scripts from patches', () => {
    const patch = formatUntrackedFileDiff(
      'src/fleet/pages/sample_widget.tsx',
      'export const SampleWidget = () => <div>Hello</div>;\n',
    );
    const noNewlinePatch = formatUntrackedFileDiff(
      'src/fleet/pages/no_eof_newline.tsx',
      'export const NoEofNewline = 1;',
    );

    expect(patch).toContain(
      'diff --git a/src/fleet/pages/sample_widget.tsx b/src/fleet/pages/sample_widget.tsx',
    );
    expect(patch).toMatch(
      /index 0000000000000000000000000000000000000000\.\.[0-9a-f]{40}/,
    );
    expect(patch).not.toContain('\\ No newline at end of file');
    expect(noNewlinePatch).toContain(
      '+export const NoEofNewline = 1;\n\\ No newline at end of file\n',
    );
    expect(
      isExcludedFromPrototypePatch('src/fleet/scripts/build_prototype.ts'),
    ).toBe(true);
    expect(
      isExcludedFromPrototypePatch('src/fleet/scripts/prototype_index.html'),
    ).toBe(true);
    expect(
      isExcludedFromPrototypePatch('src/fleet/scripts/prototype_app.tsx'),
    ).toBe(true);
    expect(
      isExcludedFromPrototypePatch('src/fleet/pages/home_page/home_page.tsx'),
    ).toBe(false);
  });

  it('writes changes.patch and metadata for prototype source artifacts', () => {
    const uiRoot = path.resolve(
      process.cwd(),
      fs.existsSync(path.join(process.cwd(), 'src', 'App.tsx'))
        ? '.'
        : 'milo/ui',
    );
    const tmpOut = fs.mkdtempSync(path.join(os.tmpdir(), 'fcon-proto-test-'));

    try {
      const meta = buildPrototypeMetadata(
        {
          slug: 'sample-proto',
          title: 'Sample Prototype',
          description: 'Verification test',
          route: '/ui/fleet/p/chromeos/devices',
        },
        '2026-10-04T00:00:00.000Z',
      );
      const artifacts = generatePrototypeSourceArtifacts(uiRoot, tmpOut, meta);

      expect(artifacts.baseCommitSha).not.toBe('');
      expect(artifacts.patchContent).toContain('# Base-Commit:');
      expect(fs.existsSync(path.join(tmpOut, 'changes.patch'))).toBe(true);
    } finally {
      fs.rmSync(tmpOut, { recursive: true, force: true });
    }
  });

  it('validates --out-dir safety and parses CLI flags', () => {
    expect(() => validateOutDir('/workspace/ui', '/workspace/ui')).toThrow(
      /Refusing to clean unsafe --out-dir/,
    );
    expect(() => validateOutDir('/workspace/ui', '/workspace')).toThrow(
      /Refusing to clean unsafe --out-dir/,
    );
    expect(() =>
      validateOutDir('/workspace/ui', '/workspace/ui/src/fleet'),
    ).toThrow(/Refusing to clean unsafe --out-dir/);
    validateOutDir('/workspace/ui', '/workspace/ui/.tmp/dist_proto');

    const opts = parseCliArgs(
      ['--slug', 'demo-mock', '--title', 'Demo Title'],
      '/workspace/ui',
    );

    expect(opts.slug).toBe('demo-mock');
    expect(opts.title).toBe('Demo Title');
    expect(opts.outDir).toBe('/workspace/ui/.tmp/dist_proto');
  });
});
