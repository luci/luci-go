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

export const black = 'rgb(0, 0, 0)';
export const white = 'rgb(255, 255, 255)';

export const blue = {
  50: 'hsl(218, 92%, 95%)',
  100: 'hsl(216, 88%, 91%)',
  200: 'hsl(217, 88%, 83%)',
  300: 'hsl(217, 89%, 76%)',
  400: 'hsl(217, 89%, 68%)',
  500: 'hsl(217, 89%, 61%)',
  600: 'hsl(214, 82%, 51%)',
  700: 'hsl(215, 79%, 46%)',
  800: 'hsl(216, 77%, 42%)',
  900: 'hsl(217, 76%, 37%)',
} as const;

export const cyan = {
  50: 'hsl(190, 74%, 94%)',
  100: 'hsl(191, 76%, 88%)',
  200: 'hsl(190, 76%, 79%)',
  300: 'hsl(190, 75%, 70%)',
  400: 'hsl(190, 75%, 60%)',
  500: 'hsl(190, 75%, 51%)',
  600: 'hsl(187, 84%, 43%)',
  700: 'hsl(186, 81%, 38%)',
  800: 'hsl(185, 88%, 30%)',
  900: 'hsl(184, 100%, 26%)',
};

export const green = {
  50: 'hsl(137, 39%, 93%)',
  100: 'hsl(137, 40%, 86%)',
  200: 'hsl(136, 40%, 76%)',
  300: 'hsl(137, 40%, 65%)',
  400: 'hsl(136, 40%, 54%)',
  500: 'hsl(136, 53%, 43%)',
  600: 'hsl(137, 65%, 34%)',
  700: 'hsl(138, 68%, 30%)',
  800: 'hsl(140, 72%, 26%)',
  900: 'hsl(142, 77%, 22%)',
} as const;

export const grey = {
  50: 'hsl(210, 17%, 98%)',
  100: 'hsl(200, 12%, 95%)',
  200: 'hsl(216, 12%, 92%)',
  300: 'hsl(220, 9%, 87%)',
  400: 'hsl(213, 7%, 76%)',
  500: 'hsl(210, 6%, 63%)',
  600: 'hsl(207, 5%, 52%)',
  700: 'hsl(213, 5%, 39%)',
  800: 'hsl(206, 6%, 25%)',
  900: 'hsl(225, 6%, 13%)',
} as const;

export const orange = {
  50: 'hsl(27, 93%, 94%)',
  100: 'hsl(26, 96%, 89%)',
  200: 'hsl(26, 96%, 80%)',
  300: 'hsl(26, 96%, 71%)',
  400: 'hsl(26, 95%, 61%)',
  500: 'hsl(26, 96%, 54%)',
  600: 'hsl(28, 92%, 47%)',
  700: 'hsl(29, 89%, 44%)',
  800: 'hsl(31, 99%, 38%)',
  900: 'hsl(33, 100%, 35%)',
} as const;

export const pink = {
  50: 'hsl(327, 85%, 95%)',
  100: 'hsl(327, 92%, 90%)',
  200: 'hsl(327, 91%, 82%)',
  300: 'hsl(327, 100%, 77%)',
  400: 'hsl(327, 100%, 69%)',
  500: 'hsl(327, 89%, 59%)',
  600: 'hsl(326, 79%, 52%)',
  700: 'hsl(325, 79%, 45%)',
  800: 'hsl(324, 94%, 37%)',
  900: 'hsl(322, 75%, 35%)',
} as const;

export const purple = {
  50: 'hsl(271, 84%, 95%)',
  100: 'hsl(272, 91%, 91%)',
  200: 'hsl(272, 91%, 83%)',
  300: 'hsl(272, 90%, 76%)',
  400: 'hsl(272, 91%, 66%)',
  500: 'hsl(272, 89%, 61%)',
  600: 'hsl(272, 78%, 55%)',
  700: 'hsl(272, 62%, 50%)',
  800: 'hsl(272, 65%, 44%)',
  900: 'hsl(272, 71%, 39%)',
} as const;

export const red = {
  50: 'hsl(5, 79%, 95%)',
  100: 'hsl(4, 81%, 90%)',
  200: 'hsl(4, 81%, 81%)',
  300: 'hsl(5, 81%, 73%)',
  400: 'hsl(5, 81%, 65%)',
  500: 'hsl(5, 81%, 56%)',
  600: 'hsl(4, 71%, 50%)',
  700: 'hsl(1, 73%, 45%)',
  800: 'hsl(1, 82%, 39%)',
  900: 'hsl(0, 84%, 35%)',
} as const;

export const yellow = {
  50: 'hsl(46, 94%, 94%)',
  100: 'hsl(45, 97%, 88%)',
  200: 'hsl(45, 96%, 78%)',
  300: 'hsl(45, 97%, 69%)',
  400: 'hsl(45, 97%, 60%)',
  500: 'hsl(45, 97%, 50%)',
  600: 'hsl(41, 100%, 49%)',
  700: 'hsl(38, 100%, 47%)',
  800: 'hsl(34, 100%, 46%)',
  900: 'hsl(31, 100%, 45%)',
} as const;

export const colors = {
  black,
  white,
  blue,
  red,
  yellow,
  pink,
  purple,
  orange,
  grey,
  cyan,
  green,
} as const;

export default colors;
