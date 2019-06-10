// Copyright 2016 The LUCI Authors.
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

package tracer

import "syscall"

// increaseClockFrequency increases the clock rate from default 15.6ms to 1ms.
//
// This makes Bruce unhappy* but this is useful on Windows to get accurate
// measurements while collecting traces without having to use the high
// frequency timer. This is NOT strictly necessary and the code will work
// just fine without but the use case for this package is sub 10ms spans, so
// it's recommended and done by default.
//
// This function is only useful on Go 1.6 and later, on Windows, since before
// the call was done automatically on startup of every Go process. See
// https://golang.org/doc/go1.6#runtime for more information.
//
// An alternative is to use QueryPerformanceCounter() but that means using a
// kernel function call instead of reading userland memory, like time.Now(),
// which calls runtime.unixnano() that reads memory at an hard coded location.
// Calling into kernel land further increases implicit synchronization, this is
// why this code prefers to increase the timer resolution than constantly
// switch into kernel mode.
//
// On the other hand, if someone wants sub-millisecond resolution,
// QueryPerformanceCounter() should be used since it can return µs resolution
// time measurement.
//
// * https://randomascii.wordpress.com/2013/07/08/windows-timer-resolution-megawatts-wasted/
func increaseClockFrequency() {
	// https://msdn.microsoft.com/en-us/library/windows/desktop/dd757624.aspx
	if d, _ := syscall.LoadDLL("winmm.dll"); d != nil {
		if p, _ := d.FindProc("timeBeginPeriod"); p != nil {
			_, _, _ = p.Call(uintptr(1))
		}
		d.Release()
	}
}

func lowerClockFrequency() {
	if d, _ := syscall.LoadDLL("winmm.dll"); d != nil {
		if p, _ := d.FindProc("timeEndPeriod"); p != nil {
			_, _, _ = p.Call(uintptr(1))
		}
		d.Release()
	}
}
