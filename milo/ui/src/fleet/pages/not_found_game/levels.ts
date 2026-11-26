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

import { LevelDef } from './types';

export const WORLD_WIDTH = 960;
export const WORLD_HEIGHT = 540;
export const TIME_LIMIT_MS = 10_000;
export const MAX_SHURIKENS = 3;

export const LEVELS: readonly LevelDef[] = [
  {
    id: '404-dojo',
    index: 0,
    name: '01 // 404 LAB',
    subtitle:
      'Fix all 5 DUTs in 10s · Z/J Manual Repair · X/K Auto-Repair (3) · S/Down Drop',
    spawn: { x: 64, y: 468 },
    platforms: [
      // Solid ground floor
      { x: 0, y: 500, w: 960, h: 40 },
      // Left "4" pass-through shelves
      { x: 150, y: 416, w: 120, h: 14, oneWay: true },
      { x: 205, y: 336, w: 96, h: 14, oneWay: true },
      // Center "0" pass-through shelves
      { x: 385, y: 412, w: 190, h: 14, oneWay: true },
      { x: 405, y: 266, w: 150, h: 14, oneWay: true },
      // Right "4" shelves (88px step rise = crisp single jump)
      { x: 650, y: 356, w: 130, h: 14, oneWay: true },
      { x: 755, y: 268, w: 145, h: 14, oneWay: true },
    ],
    hazards: [],
    devices: [
      { id: 'd1', x: 235, y: 472, kind: 'chromebook' },
      { id: 'd2', x: 253, y: 304, kind: 'pixel' },
      { id: 'd3', x: 480, y: 378, kind: 'server' },
      // d4 + d5 align at y=234 for a double Auto-Repair line!
      { id: 'd4', x: 480, y: 234, kind: 'pixel' },
      { id: 'd5', x: 830, y: 234, kind: 'chromebook' },
    ],
    hints: [
      { x: 112, y: 292, text: 'A/D Move · W/Space Double-Jump' },
      { x: 148, y: 466, text: 'Z / J: Manual Repair' },
      { x: 655, y: 202, text: 'X / K: Auto-Repair pierces aligned DUTs!' },
    ],
    stars: {
      tasTimeMs: 1167,
      devTimeMs: 2017,
      threeStarMs: 2700,
      twoStarMs: 4500,
      oneStarMs: TIME_LIMIT_MS,
    },
  },
  {
    id: 'pierce-puzzle',
    index: 1,
    name: '02 // REPAIR CHAIN',
    subtitle:
      'Manual-Repair center DUT to unlock mid-lane Auto-Repair · Clear all 6 DUTs fast!',
    spawn: { x: 466, y: 468 },
    platforms: [
      // Solid ground floor
      { x: 0, y: 500, w: 960, h: 40 },
      // Center shelves
      { x: 400, y: 374, w: 160, h: 14, oneWay: true },
      { x: 412, y: 248, w: 136, h: 14, oneWay: true },
      // Wing shelves (upper right shelf 145px from center for clean aerial chain!)
      { x: 75, y: 374, w: 185, h: 14, oneWay: true },
      { x: 700, y: 374, w: 185, h: 14, oneWay: true },
      { x: 140, y: 248, w: 175, h: 14, oneWay: true },
      { x: 580, y: 248, w: 180, h: 14, oneWay: true },
    ],
    hazards: [],
    devices: [
      // Floor Auto-Repair pair (y=472)
      { id: 'd1', x: 112, y: 472, kind: 'chromebook' },
      { id: 'd2', x: 848, y: 472, kind: 'chromebook' },
      // Mid-lane ORDER PUZZLE: NEEDS_MANUAL_REPAIR blocks Auto-Repair until fixed!
      { id: 'd3', x: 155, y: 342, kind: 'pixel' },
      { id: 'd4', x: 480, y: 342, kind: 'server', shielded: true },
      { id: 'd5', x: 805, y: 342, kind: 'pixel' },
      // Upper right target — directly linked via aerial Manual Repair jump-reset off d4!
      { id: 'd6', x: 635, y: 214, kind: 'server' },
    ],
    hints: [
      { x: 480, y: 446, text: 'Manual Repair center -> unlocks Auto-Repair!' },
    ],
    stars: {
      tasTimeMs: 1267,
      devTimeMs: 2533,
      threeStarMs: 3200,
      twoStarMs: 5000,
      oneStarMs: TIME_LIMIT_MS,
    },
  },
  {
    id: 'rack-shaft',
    index: 2,
    name: '03 // RACK SHAFT',
    subtitle:
      'Wall-jump server columns over power faults · Fire Auto-Repair through rack gaps',
    spawn: { x: 466, y: 468 },
    platforms: [
      // Solid ground floor
      { x: 0, y: 500, w: 960, h: 40 },
      // Left solid server column (split with 2 horizontal firing windows!)
      { x: 300, y: 152, w: 22, h: 72 },
      { x: 300, y: 286, w: 22, h: 98 },
      // Right solid server column (split with 2 horizontal firing windows!)
      { x: 638, y: 152, w: 22, h: 72 },
      { x: 638, y: 286, w: 22, h: 98 },
      // Center one-way shelves (96px single-jump rise from 286 -> 190)
      { x: 396, y: 400, w: 168, h: 14, oneWay: true },
      { x: 396, y: 286, w: 168, h: 14, oneWay: true },
      { x: 382, y: 190, w: 196, h: 14, oneWay: true },
      // Outer wing perches aligned with firing windows
      { x: 86, y: 446, w: 148, h: 14, oneWay: true },
      { x: 726, y: 446, w: 148, h: 14, oneWay: true },
      { x: 96, y: 286, w: 152, h: 14, oneWay: true },
      { x: 712, y: 286, w: 152, h: 14, oneWay: true },
    ],
    hazards: [
      // Floor power-fault beams flanking spawn so wall-jumping pillars matters!
      { x: 256, y: 486, w: 120, h: 14 },
      { x: 584, y: 486, w: 120, h: 14 },
    ],
    devices: [
      { id: 'd1', x: 148, y: 414, kind: 'chromebook' },
      { id: 'd2', x: 812, y: 414, kind: 'chromebook' },
      { id: 'd3', x: 480, y: 368, kind: 'server', shielded: true },
      { id: 'd4', x: 168, y: 254, kind: 'pixel' },
      { id: 'd5', x: 792, y: 254, kind: 'pixel' },
      { id: 'd6', x: 480, y: 158, kind: 'server', shielded: true },
    ],
    hints: [
      { x: 168, y: 344, text: 'Auto-Repair flies through rack gaps!' },
      { x: 480, y: 234, text: 'S / Down drops fast through shelves' },
    ],
    stars: {
      tasTimeMs: 1517,
      devTimeMs: 2733,
      threeStarMs: 3500,
      twoStarMs: 5500,
      oneStarMs: TIME_LIMIT_MS,
    },
  },
  {
    id: 'laser-lab',
    index: 3,
    name: '04 // LASER LAB',
    subtitle:
      'Dodge red power-fault beams · Align Auto-Repair shots across hazard pits',
    spawn: { x: 56, y: 388 },
    platforms: [
      // Solid start deck
      { x: 20, y: 420, w: 150, h: 24 },
      // Mid one-way islands
      { x: 240, y: 374, w: 145, h: 14, oneWay: true },
      { x: 455, y: 324, w: 155, h: 14, oneWay: true },
      { x: 455, y: 444, w: 155, h: 14, oneWay: true },
      // Far right shelves + solid wall-jump column
      { x: 690, y: 388, w: 150, h: 14, oneWay: true },
      { x: 912, y: 162, w: 24, h: 256 },
      { x: 720, y: 238, w: 165, h: 14, oneWay: true },
      // Solid floor
      { x: 0, y: 512, w: 960, h: 28 },
    ],
    hazards: [
      { x: 176, y: 498, w: 270, h: 14 },
      { x: 618, y: 498, w: 280, h: 14 },
      { x: 646, y: 242, w: 14, h: 118 },
    ],
    devices: [
      // d1 (y=342) and d4 (y=356) line up from island 1 for a clean double Auto-Repair!
      { id: 'd1', x: 312, y: 342, kind: 'chromebook' },
      { id: 'd2', x: 532, y: 292, kind: 'pixel' },
      { id: 'd3', x: 532, y: 412, kind: 'server' },
      { id: 'd4', x: 765, y: 356, kind: 'chromebook' },
      { id: 'd5', x: 805, y: 206, kind: 'server', shielded: true },
      { id: 'd6', x: 312, y: 206, kind: 'pixel' },
    ],
    hints: [
      { x: 312, y: 472, text: 'Avoid red power-fault beams!' },
      { x: 786, y: 278, text: 'Wall-jump right wall!' },
    ],
    stars: {
      tasTimeMs: 1783,
      devTimeMs: 2850,
      threeStarMs: 3700,
      twoStarMs: 5800,
      oneStarMs: TIME_LIMIT_MS,
    },
  },
  {
    id: 'ten-second-fleet',
    index: 4,
    name: '05 // 10s FLEET',
    subtitle:
      '7 broken DUTs · Chain aerial Manual Repairs, Auto-Repair lines & bottom tunnel skips',
    spawn: { x: 62, y: 142 },
    platforms: [
      // Top-left solid launch balcony
      { x: 20, y: 174, w: 155, h: 20 },
      // Upper one-way bridges (trimmed clear of d7)
      { x: 300, y: 174, w: 136, h: 14, oneWay: true },
      { x: 750, y: 174, w: 180, h: 14, oneWay: true },
      // Mid one-way ledges + shortened center pillar (144px bottom tunnel clearance!)
      { x: 120, y: 334, w: 165, h: 14, oneWay: true },
      { x: 468, y: 224, w: 24, h: 132 },
      { x: 610, y: 334, w: 175, h: 14, oneWay: true },
      // Solid ground floor
      { x: 0, y: 500, w: 960, h: 40 },
    ],
    hazards: [{ x: 412, y: 486, w: 136, h: 14 }],
    devices: [
      // Frame-tight trick: walking off balcony & firing Auto-Repair mid-drop pierces d1+d7+d2!
      { id: 'd1', x: 368, y: 142, kind: 'pixel' },
      { id: 'd2', x: 845, y: 142, kind: 'chromebook' },
      { id: 'd3', x: 202, y: 302, kind: 'server', shielded: true },
      { id: 'd4', x: 695, y: 302, kind: 'server', shielded: true },
      { id: 'd5', x: 155, y: 472, kind: 'chromebook' },
      { id: 'd6', x: 810, y: 472, kind: 'pixel' },
      { id: 'd7', x: 480, y: 188, kind: 'chromebook' },
    ],
    hints: [{ x: 480, y: 450, text: 'Final test: 7 DUTs in 10 seconds!' }],
    stars: {
      tasTimeMs: 1483,
      devTimeMs: 2500,
      threeStarMs: 3400,
      twoStarMs: 5400,
      oneStarMs: TIME_LIMIT_MS,
    },
  },
];
