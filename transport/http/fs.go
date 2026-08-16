package http

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type (
	filesystem struct {
		fs            http.FileSystem
		modtime       time.Time
		prefix        string
		indexFile     string
		denyDirectory bool
	}

	httpFile struct {
		fp      http.File
		modtime time.Time
	}

	httpFileInfo struct {
		name    string
		size    int64
		mode    fs.FileMode
		isDir   bool
		modtime time.Time
	}
)

func (fi *httpFileInfo) Name() string {
	return fi.name
}

func (fi *httpFileInfo) Size() int64 {
	return fi.size
}

func (fi *httpFileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi *httpFileInfo) ModTime() time.Time {
	return fi.modtime
}

func (fi *httpFileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *httpFileInfo) Sys() any {
	return nil
}

func (file *httpFile) Close() error {
	return file.fp.Close()
}

func (file *httpFile) Read(p []byte) (n int, err error) {
	return file.fp.Read(p)
}

func (file *httpFile) Seek(offset int64, whence int) (int64, error) {
	return file.fp.Seek(offset, whence)
}

func (file *httpFile) Readdir(count int) ([]fs.FileInfo, error) {
	return file.fp.Readdir(count)
}

func (file *httpFile) Stat() (fs.FileInfo, error) {
	fi, err := file.fp.Stat()
	if err != nil {
		return nil, err
	}
	return newFileInfo(fi, file.modtime), nil
}

func (fs *filesystem) DenyAccessDirectory() {
	fs.denyDirectory = true
}

func (fs *filesystem) SetPrefix(prefix string) {
	if prefix != "" {
		if prefix[0] != '/' {
			prefix = "/" + prefix
		}
		prefix = strings.TrimRight(prefix, "/")
		fs.prefix = prefix
	}
}

func (fs *filesystem) SetIndexFile(indexFile string) {
	fs.indexFile = indexFile
}

func (fs *filesystem) Open(name string) (http.File, error) {
	var (
		needRetry bool
	)
	name = path.Clean(name)
	if name == "" || name == "/" {
		needRetry = true
	}
	if fs.prefix != "" {
		if !strings.HasPrefix(name, fs.prefix) {
			name = path.Join(fs.prefix, name)
		}
	}
	fp, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	if fs.denyDirectory {
		state, err := fp.Stat()
		if err != nil {
			return nil, err
		}
		if state.IsDir() {
			if needRetry {
				if fs.indexFile != "" {
					return fs.Open(path.Join(name, fs.indexFile))
				}
			}
			return nil, os.ErrPermission
		}
	}
	return &httpFile{fp: fp, modtime: fs.modtime}, nil
}

func newFS(modtime time.Time, fs http.FileSystem) *filesystem {
	return &filesystem{
		fs:      fs,
		modtime: modtime,
	}
}

func newFileInfo(fi fs.FileInfo, modtime time.Time) *httpFileInfo {
	return &httpFileInfo{
		name:    fi.Name(),
		size:    fi.Size(),
		mode:    fi.Mode(),
		isDir:   fi.IsDir(),
		modtime: modtime,
	}
}

func FileSystem(s fs.FS) http.FileSystem {
	return http.FS(s)
}
