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

const toSvgUri = (slug: string, defs: string, body: string): string =>
  `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 450" ' +
      `preserveAspectRatio="xMidYMid slice"><defs>${defs}</defs>${body}</svg>`,
  )}#${slug}`;

const vGrad = (id: string, stops: string): string =>
  `<linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1">${stops
    .split(' ')
    .map((s) => s.split(':'))
    .map(([o, c]) => `<stop offset="${o}%" stop-color="${c}"/>`)
    .join('')}</linearGradient>`;

const slots = (x: number, y: number, w: number, h: number, n: number) =>
  Array.from({ length: n }, (_, i) => {
    const ry = +(y + 8 + i * ((h - 16) / n)).toFixed(1);
    const c = i % 5 === 2 ? '#4285f4' : i === n - 2 ? '#fbbc04' : '#34a853';
    return (
      `<rect x="${x + 6}" y="${ry}" width="${w - 12}" height="5" ` +
      `rx="1" fill="#0f172a" opacity="0.72"/>` +
      `<circle cx="${x + w - 12}" cy="${(ry + 2.5).toFixed(1)}" r="2" fill="${c}"/>`
    );
  }).join('');

const RACK_AISLE = toSvgUri(
  'mock-rack-aisle.svg',
  vGrad('bg', '0:#e2e8f0 56:#cbd5e1 100:#f1f5f9') +
    vGrad('fl', '0:#94a3b8 100:#e2e8f0'),
  '<rect width="800" height="450" fill="url(#bg)"/>' +
    '<polygon points="260,315 540,315 800,450 0,450" fill="url(#fl)"/>' +
    '<polygon points="318,315 482,315 620,450 180,450" fill="#f8fafc" opacity="0.52"/>' +
    '<rect x="330" y="138" width="140" height="177" fill="#dbeafe" stroke="#64748b" stroke-width="3"/>' +
    '<rect x="366" y="158" width="68" height="145" fill="#eff6ff" opacity="0.85"/>' +
    '<polygon points="290,0 510,0 468,138 332,138" fill="#cbd5e1"/>' +
    '<polygon points="362,0 438,0 422,138 378,138" fill="#ffffff" opacity="0.9"/>' +
    '<rect x="295" y="42" width="210" height="10" rx="2" fill="#1a73e8" opacity="0.82"/>' +
    '<polygon points="0,24 238,116 238,354 0,450" fill="#1e293b"/>' +
    '<polygon points="238,116 330,138 330,315 238,354" fill="#334155"/>' +
    '<polygon points="800,24 562,116 562,354 800,450" fill="#1e293b"/>' +
    '<polygon points="562,116 470,138 470,315 562,354" fill="#334155"/>' +
    '24,64,92,330,12 132,96,84,270,10 684,64,92,330,12 584,96,84,270,10'
      .split(' ')
      .map((s) => s.split(',').map(Number))
      .map(
        ([x, y, w, h, n]) =>
          `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="4" ` +
          `fill="#273449" stroke="#475569" stroke-width="2"/>${slots(x, y, w, h, n)}`,
      )
      .join(''),
);

const CAMPUS_EXTERIOR = toSvgUri(
  'mock-campus-exterior.svg',
  vGrad('sky', '0:#93c5fd 68:#e0f2fe 100:#fef3c7') +
    vGrad('gl', '0:#60a5fa 52:#bae6fd 100:#3b82f6'),
  '<rect width="800" height="450" fill="url(#sky)"/>' +
    '<rect y="346" width="800" height="104" fill="#cbd5e1"/>' +
    '<rect x="74" y="106" width="652" height="240" rx="4" fill="#f8fafc" stroke="#94a3b8" stroke-width="2"/>' +
    '<rect x="56" y="92" width="688" height="22" rx="3" fill="#334155"/>' +
    '<rect x="100" y="128" width="440" height="218" fill="url(#gl)" opacity="0.72" stroke="#64748b" stroke-width="2"/>' +
    '<rect x="558" y="128" width="144" height="218" fill="#e2e8f0" stroke="#94a3b8" stroke-width="2"/>' +
    '<rect x="588" y="250" width="84" height="96" fill="#1e293b" opacity="0.82"/>' +
    Array.from(
      { length: 7 },
      (_, i) =>
        `<rect x="${100 + i * 63}" y="128" width="2" height="218" fill="#f8fafc" opacity="0.8"/>`,
    ).join('') +
    '<rect x="100" y="198" width="440" height="4" fill="#334155"/>' +
    '<rect x="100" y="272" width="440" height="4" fill="#334155"/>' +
    '<ellipse cx="96" cy="348" rx="68" ry="26" fill="#65a30d" opacity="0.72"/>' +
    '<ellipse cx="708" cy="348" rx="74" ry="28" fill="#4d7c0f" opacity="0.7"/>',
);

const DEVICE_BAY = toSvgUri(
  'mock-device-bay.svg',
  vGrad('wall', '0:#f8fafc 100:#e2e8f0'),
  '<rect width="800" height="450" fill="url(#wall)"/>' +
    '<rect y="388" width="800" height="62" fill="#cbd5e1"/>' +
    '<rect x="110" y="24" width="580" height="10" rx="5" fill="#ffffff" stroke="#cbd5e1"/>' +
    [90, 315, 540]
      .map(
        (x) =>
          `<rect x="${x}" y="68" width="170" height="320" rx="6" fill="#1e293b" stroke="#475569" stroke-width="3"/>` +
          `<rect x="${x + 10}" y="80" width="150" height="296" rx="3" fill="#0f172a"/>` +
          `<rect x="${x + 150}" y="86" width="6" height="282" rx="2" fill="#1a73e8"/>` +
          slots(x + 14, 84, 130, 286, 11),
      )
      .join(''),
);

const HARDWARE_BENCH = toSvgUri(
  'mock-hardware-bench.svg',
  vGrad('room', '0:#f1f5f9 100:#dbeafe'),
  '<rect width="800" height="450" fill="url(#room)"/>' +
    '<rect x="44" y="48" width="712" height="186" rx="4" fill="#ffffff" opacity="0.72" stroke="#cbd5e1" stroke-width="2"/>' +
    '<rect y="330" width="800" height="120" fill="#cbd5e1"/>' +
    [82, 426]
      .map(
        (x) =>
          `<rect x="${x}" y="256" width="292" height="18" rx="3" fill="#475569"/>` +
          `<rect x="${x + 18}" y="274" width="14" height="114" fill="#64748b"/>` +
          `<rect x="${x + 260}" y="274" width="14" height="114" fill="#64748b"/>` +
          `<rect x="${x + 28}" y="176" width="108" height="68" rx="4" fill="#1e293b" stroke="#94a3b8" stroke-width="2"/>` +
          `<rect x="${x + 36}" y="184" width="92" height="50" fill="#1a73e8" opacity="0.24"/>` +
          `<rect x="${x + 156}" y="218" width="110" height="38" rx="3" fill="#334155"/>` +
          `<circle cx="${x + 252}" cy="236" r="3" fill="#34a853"/>`,
      )
      .join(''),
);

const LAB_WING = toSvgUri(
  'mock-lab-wing.svg',
  vGrad('wsky', '0:#bfdbfe 70:#fef3c7 100:#fde68a'),
  '<rect width="800" height="450" fill="url(#wsky)"/>' +
    '<rect y="352" width="800" height="98" fill="#d6d3d1"/>' +
    '<rect x="96" y="118" width="608" height="234" rx="4" fill="#f5f5f4" stroke="#a8a29e" stroke-width="2"/>' +
    '<rect x="96" y="118" width="206" height="234" fill="#e7e5e4"/>' +
    '<rect x="320" y="142" width="356" height="210" fill="#0f172a" opacity="0.72"/>' +
    Array.from(
      { length: 5 },
      (_, i) =>
        `<rect x="${332 + i * 68}" y="156" width="54" height="184" fill="#bae6fd" opacity="0.58"/>`,
    ).join('') +
    '<ellipse cx="144" cy="356" rx="82" ry="26" fill="#15803d" opacity="0.7"/>',
);

export const MOCK_SITE_PHOTOS = {
  rackAisle: RACK_AISLE,
  campusExterior: CAMPUS_EXTERIOR,
  deviceBay: DEVICE_BAY,
  hardwareBench: HARDWARE_BENCH,
  labWing: LAB_WING,
} as const;
