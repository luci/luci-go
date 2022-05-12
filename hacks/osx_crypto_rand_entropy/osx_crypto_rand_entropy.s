// Copyright 2022 The LUCI Authors.
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

//+build darwin,amd64,go1.17

#include "textflag.h"

// Need to provide some implementation, otherwise the compiled binary would
// still dynamically link to `getentropy` in libSystem.B.dylib, which is absent
// on OSX 10.11.
//
// The only caller in stdlib is crypto/rand, which we hack separately to avoid
// calling this function.
TEXT libc_getentropy(SB),NOSPLIT,$0-0
    INT $3
