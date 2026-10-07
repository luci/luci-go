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
  buildEasyMockAppModule,
  buildPrototypeMetadata,
  formatUntrackedFileDiff,
  generateOfflineSettingsJs,
  generatePrototypeSourceArtifacts,
  isExcludedFromPrototypePatch,
  parseCliArgs,
  stripCrossOriginAttributes,
  validateOutDir,
} from './build_prototype';

describe('build_prototype utilities', () => {
  it('strips crossorigin attributes and Trusted Types CSP meta tags from HTML', () => {
    const rawHtml = [
      '<meta http-equiv="Content-Security-Policy" content="require-trusted-types-for \'script\';">',
      '<script type="module" crossorigin src="./assets/fleet.js"></script>',
      '<link rel="stylesheet" crossorigin="anonymous" href="./assets/fleet.css">',
    ].join('');
    const cleaned = stripCrossOriginAttributes(rawHtml);
    expect(cleaned).not.toContain('crossorigin');
    expect(cleaned).not.toContain('Content-Security-Policy');
    expect(cleaned).toContain('src="./assets/fleet.js"');
  });

  it('builds a self-contained Easy Mock App module with createMemoryRouter and hash sync', () => {
    const mod = buildEasyMockAppModule();
    expect(mod).toContain('FleetConsoleMockAPI.initPrototypeMode(');
    expect(mod).toContain('createMemoryRouter(');
    expect(mod).toContain("window.addEventListener('hashchange', onHash)");
    expect(mod).toContain("window.addEventListener('popstate', onHash)");
  });

  it('formats untracked new files with 40-char blob SHAs and excludes builder paths from changes.patch', () => {
    const patch = formatUntrackedFileDiff(
      'src/fleet/pages/sample_widget.tsx',
      'export const SampleWidget = () => <div>Hello</div>;\n',
    );
    expect(patch).toContain(
      'diff --git a/src/fleet/pages/sample_widget.tsx b/src/fleet/pages/sample_widget.tsx',
    );
    expect(patch).toMatch(
      /index 0000000000000000000000000000000000000000\.\.[0-9a-f]{40}/,
    );
    expect(patch).not.toContain('\\ No newline at end of file');

    const noNewlinePatch = formatUntrackedFileDiff(
      'src/fleet/pages/no_eof_newline.tsx',
      'export const NoEofNewline = 1;',
    );
    expect(noNewlinePatch).toContain(
      '+export const NoEofNewline = 1;\n\\ No newline at end of file\n',
    );
    expect(
      isExcludedFromPrototypePatch('src/fleet/scripts/build_prototype.ts'),
    ).toBe(true);
    expect(
      isExcludedFromPrototypePatch('src/fleet/pages/home_page/home_page.tsx'),
    ).toBe(false);

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
      expect(artifacts.baseCommitSha).toBeTruthy();
      expect(artifacts.patchContent).toContain('# Base-Commit:');
      expect(fs.existsSync(path.join(tmpOut, 'changes.patch'))).toBe(true);
    } finally {
      fs.rmSync(tmpOut, { recursive: true, force: true });
    }
  });

  it('generates offline settings.js and validates --out-dir safety', () => {
    const settingsJs = generateOfflineSettingsJs('/ui/fleet/catalog');
    expect(settingsJs).toContain('window.__FCON_EASY_MOCK__ = true;');
    expect(settingsJs).toContain("cleanUrl.hash = '#' + normalized;");

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
    expect(opts.outDir).toBe('/workspace/ui/.tmp/dist_proto');
  });
});
