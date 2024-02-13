// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/system/environ"

	"google.golang.org/protobuf/proto"
)

// ActionFilesCopyTransformer is the default transformer for
// core.ActionFilesCopy.
func ActionFilesCopyTransformer(a *core.ActionFilesCopy, deps []Package) (*core.Derivation, error) {
	drv, err := ReexecDerivation(a, false)
	if err != nil {
		return nil, err
	}

	// Clone the action so we can remove path when regenerating the hash.
	// If version is set, we don't need path to determine whether content
	// changed. Recalculate the hash based on the assumption.
	m := proto.Clone(a).(*core.ActionFilesCopy)
	for _, f := range m.Files {
		if l := f.GetLocal(); l.GetVersion() != "" && fs.FileMode(f.Mode).Type() != fs.ModeSymlink {
			l.Path = ""
		}
	}
	if drv.FixedOutput, err = sha256String(m); err != nil {
		return nil, err
	}

	for _, d := range deps {
		name := d.Action.Name
		outDir := d.Handler.OutputDirectory()
		drv.Env = append(drv.Env, fmt.Sprintf("%s=%s", name, outDir))
	}
	return drv, nil
}

var defaultFilesCopyExecutor = newFilesCopyExecutor()

// RegisterEmbed regists the embedded fs with ref. It can be retrieved by copy
// actions using embed source. Embedded fs need to be registered in init() for
// re-exec executor.
func RegisterEmbed(ref string, e embed.FS) {
	defaultFilesCopyExecutor.StoreEmbed(ref, e)
}

// FilesCopyExecutor is the default executor for core.ActionFilesCopy.
// All embed.FS must be registered in init() so they are available when being
// executed from reexec.
type filesCopyExecutor struct {
	sealed bool
	embeds map[string]embed.FS
}

func newFilesCopyExecutor() *filesCopyExecutor {
	return &filesCopyExecutor{
		embeds: make(map[string]embed.FS),
	}
}

// StoreEmbed stores an embedded fs for copy executor. This need to be called
// before main function otherwise executor may not be able to load the fs.
// Panic if the same ref is stored more than once.
func (f *filesCopyExecutor) StoreEmbed(ref string, e embed.FS) {
	if f.sealed {
		panic("all embedded fs must be stored before use")
	}
	if _, ok := f.embeds[ref]; ok {
		panic(fmt.Sprintf("embedded fs with ref: \"%s\" registed twice", ref))
	}
	f.embeds[ref] = e
}

// LoadEmbed load a stored embedded fs.
func (f *filesCopyExecutor) LoadEmbed(ref string) (embed.FS, bool) {
	f.sealed = true
	e, ok := f.embeds[ref]
	return e, ok
}

// Execute copies the files listed in the core.ActionFilesCopy to the out
// directory provided.
func (f *filesCopyExecutor) Execute(ctx context.Context, a *core.ActionFilesCopy, out string) error {
	for dst, srcFile := range a.Files {
		dst = filepath.Join(out, dst)
		if err := os.MkdirAll(filepath.Dir(dst), fs.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory: %s: %w", path.Base(dst), err)
		}
		m := fs.FileMode(srcFile.Mode)

		switch c := srcFile.Content.(type) {
		case *core.ActionFilesCopy_Source_Raw:
			if err := copyRaw(c.Raw, dst, m); err != nil {
				return err
			}
		case *core.ActionFilesCopy_Source_Local_:
			if err := copyLocal(c.Local, dst, m); err != nil {
				return err
			}
		case *core.ActionFilesCopy_Source_Embed_:
			if err := f.copyEmbed(c.Embed, dst, m); err != nil {
				return err
			}
		case *core.ActionFilesCopy_Source_Output_:
			if err := copyOutput(c.Output, environ.FromCtx(ctx), dst, m); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type for %s: %s", dst, m.Type())
		}
	}
	return nil
}

func copyRaw(raw []byte, dst string, m fs.FileMode) error {
	switch m.Type() {
	case fs.ModeSymlink:
		return fmt.Errorf("symlink is not supported for the source type")
	case fs.ModeDir:
		if err := os.MkdirAll(dst, m); err != nil {
			return fmt.Errorf("failed to create directory: %s: %w", path.Base(dst), err)
		}
		return nil
	case 0: // Regular File
		if err := createFile(dst, m, bytes.NewReader(raw)); err != nil {
			return fmt.Errorf("failed to create file: %s: %w", dst, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown file type for %s: %s", dst, m.Type())
	}
}

func copyLocal(s *core.ActionFilesCopy_Source_Local, dst string, m fs.FileMode) error {
	src := s.Path
	switch m.Type() {
	case fs.ModeSymlink:
		if s.FollowSymlinks {
			return fmt.Errorf("invalid file spec: followSymlinks can't be used with symlink dst: %s", dst)
		}
		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("failed to create symlink: %s -> %s: %w", dst, src, err)
		}
		return nil
	case fs.ModeDir:
		return copyFS(os.DirFS(src), s.FollowSymlinks, dst)
	case 0: // Regular File
		f, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("failed to open source file for %s: %s: %w", dst, src, err)
		}
		defer f.Close()
		if err := createFile(dst, m, f); err != nil {
			return fmt.Errorf("failed to create file for %s: %s: %w", dst, src, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown file type for %s: %s", dst, m.Type())
	}
}

func (f *filesCopyExecutor) copyEmbed(s *core.ActionFilesCopy_Source_Embed, dst string, m fs.FileMode) error {
	e, ok := f.LoadEmbed(s.Ref)
	if !ok {
		return fmt.Errorf("failed to load embedded fs for %s: %s", dst, s)
	}

	switch m.Type() {
	case fs.ModeSymlink:
		return fmt.Errorf("symlink not supported for the source type")
	case fs.ModeDir:
		path := s.Path
		if path == "" {
			path = "."
		}
		src, err := fs.Sub(e, path)
		if err != nil {
			return fmt.Errorf("failed to load subdir fs for %s: %s: %w", dst, s, err)
		}
		return copyFS(src, true, dst)
	case 0: // Regular File
		f, err := e.Open(s.Path)
		if err != nil {
			return fmt.Errorf("failed to open source file for %s: %s: %w", dst, s, err)
		}
		defer f.Close()
		if err := createFile(dst, m, f); err != nil {
			return fmt.Errorf("failed to create file for %s: %s: %w", dst, s, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown file type for %s: %s", dst, m.Type())
	}
}

func copyOutput(s *core.ActionFilesCopy_Source_Output, env environ.Env, dst string, m fs.FileMode) error {
	out := env.Get(s.Name)
	if out == "" {
		return fmt.Errorf("output not found: %s: %s", dst, s)
	}
	return copyLocal(&core.ActionFilesCopy_Source_Local{
		Path:           filepath.Join(out, s.Path),
		FollowSymlinks: false,
	}, dst, m)
}

func copyFS(src fs.FS, followSymlinks bool, dst string) error {
	return fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		log.Printf("copying %s\n", name)

		dstName := filepath.Join(dst, filepath.FromSlash(name))

		var cerr error
		err = func() error {
			switch {
			case d.Type() == fs.ModeSymlink && !followSymlinks:
				src, ok := src.(ReadLinkFS)
				if !ok {
					return fmt.Errorf("readlink not supported on the source filesystem: %s", name)
				}
				target, err := src.ReadLink(name)
				if err != nil {
					return fmt.Errorf("failed to readlink: %s: %w", name, err)
				}
				if filepath.IsAbs(target) {
					return fmt.Errorf("absolute symlink target not supported: %s", name)
				}

				if err := os.Symlink(target, dstName); err != nil {
					return fmt.Errorf("failed to create symlink: %s -> %s: %w", name, target, err)
				}
			case d.Type() == fs.ModeDir:
				if err := os.MkdirAll(dstName, fs.ModePerm); err != nil {
					return fmt.Errorf("failed to create dir: %s: %w", name, err)
				}
				return nil
			default: // Regular File or following symlinks
				srcFile, err := src.Open(name)
				if err != nil {
					return fmt.Errorf("failed to open src file: %s: %w", name, err)
				}
				defer func() { cerr = errors.Join(cerr, srcFile.Close()) }()

				info, err := fs.Stat(src, name)
				if err != nil {
					return fmt.Errorf("failed to get file mode: %s: %w", name, err)
				}

				if err := createFile(dstName, info.Mode(), srcFile); err != nil {
					return fmt.Errorf("failed to create file: %s: %w", name, err)
				}
			}
			return nil
		}()

		return errors.Join(err, cerr)
	})
}

func createFile(dst string, m fs.FileMode, r io.Reader) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := os.Chmod(dst, m); err != nil {
		return err
	}
	return nil
}
