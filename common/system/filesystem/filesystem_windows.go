// Copyright 2019 The LUCI Authors.
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

package filesystem

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"

	"go.chromium.org/luci/common/errors"
)

func umask(mask int) int {
	return 0
}

func addReadMode(mode os.FileMode) os.FileMode {
	return mode | syscall.S_IRUSR
}

func getProcessToken() (windows.Token, error) {
	var token windows.Token
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return token, errors.Annotate(err, "failed to get current process").Err()
	}

	if err := windows.OpenProcessToken(process, windows.TOKEN_ALL_ACCESS, &token); err != nil {
		return token, errors.Annotate(err, "failed to open process token").Err()
	}
	return token, nil
}

func getLUID(name string) (windows.LUID, error) {
	var luid windows.LUID
	utf16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return luid, errors.Annotate(err, `failed to convert "%s" to utf16`, name).Err()
	}

	if err := windows.LookupPrivilegeValue(nil, utf16, &luid); err != nil {
		return luid, errors.Annotate(err, "failed to lookup privilege value for %s", name).Err()
	}
	return luid, nil
}

func enableSymlink() error {
	luid, err := getLUID("SeCreateSymbolicLinkPrivilege")
	if err != nil {
		return errors.Annotate(err, "failed to get LUID for SeCreateSymbolicLinkPrivilege").Err()
	}
	privileges := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{
				Luid:       luid,
				Attributes: windows.SE_PRIVILEGE_ENABLED,
			},
		},
	}

	token, err := getProcessToken()
	if err != nil {
		return errors.Annotate(err, "failed to get process token").Err()
	}
	defer windows.CloseHandle(windows.Handle(token))

	if err := windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil); err != nil {
		return errors.Annotate(err, "failed to adjust token privileges").Err()
	}

	return nil
}
