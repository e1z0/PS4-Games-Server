// SPDX-License-Identifier: GPL-2.0-only
// Release archive creation uses Go alone, including zip support for Windows.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

func main() {
	v := flag.String("version", "0.1.0-dev", "release version")
	flag.Parse()
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(*v) {
		panic("invalid version")
	}
	if e := release(*v); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func release(version string) error {
	if e := os.MkdirAll("dist", 0755); e != nil {
		return e
	}
	for _, target := range []struct{ os, arch string }{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		base := "ps4-games-server-" + version + "-" + target.os + "-" + target.arch
		dir, e := os.MkdirTemp("", "ps4-release-*")
		if e != nil {
			return e
		}
		binary := "ps4-server"
		if target.os == "windows" {
			binary += ".exe"
		}
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", filepath.Join(dir, binary), "./cmd/ps4-server")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			os.RemoveAll(dir)
			return e
		}
		files := map[string]string{binary: filepath.Join(dir, binary), "config.example.json": "config.example.json", "LICENSE": "LICENSE", "SETUP.md": "docs/setup.md", "THIRD_PARTY_NOTICES.md": "THIRD_PARTY_NOTICES.md"}
		ext := ".tar.gz"
		if target.os == "windows" {
			ext = ".zip"
		}
		out, e := os.Create(filepath.Join("dist", base+ext))
		if e != nil {
			os.RemoveAll(dir)
			return e
		}
		if target.os == "windows" {
			e = writeZip(out, files, binary)
		} else {
			e = writeTar(out, files, binary)
		}
		closeErr := out.Close()
		os.RemoveAll(dir)
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println(base + ext)
	}
	return nil
}
func writeZip(w io.Writer, files map[string]string, binary string) error {
	z := zip.NewWriter(w)
	for _, name := range []string{binary, "config.example.json", "LICENSE", "SETUP.md", "THIRD_PARTY_NOTICES.md"} {
		b, e := os.ReadFile(files[name])
		if e != nil {
			return e
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		h.SetMode(0644)
		if name == binary {
			h.SetMode(0755)
		}
		f, e := z.CreateHeader(h)
		if e != nil {
			return e
		}
		if _, e = f.Write(b); e != nil {
			return e
		}
	}
	return z.Close()
}
func writeTar(w io.Writer, files map[string]string, binary string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, name := range []string{binary, "config.example.json", "LICENSE", "SETUP.md", "THIRD_PARTY_NOTICES.md"} {
		b, e := os.ReadFile(files[name])
		if e != nil {
			return e
		}
		mode := int64(0644)
		if name == binary {
			mode = 0755
		}
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(b)), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		if _, e = tw.Write(b); e != nil {
			return e
		}
	}
	if e := tw.Close(); e != nil {
		return e
	}
	return gz.Close()
}
