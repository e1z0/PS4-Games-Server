// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAtomicConcurrentReaders(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	a := bytes.Repeat([]byte("a"), 10000)
	b := bytes.Repeat([]byte("b"), 20000)
	if e := Write(p, a, 0600); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	errs := make(chan string, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				v, e := os.ReadFile(p)
				if e != nil || !bytes.Equal(v, a) && !bytes.Equal(v, b) {
					select {
					case errs <- "reader observed partial atomic write":
					default:
					}
					return
				}
			}
		}
	}()
	for i := 0; i < 20; i++ {
		var v []byte
		if i%2 == 0 {
			v = b
		} else {
			v = a
		}
		if e := Write(p, v, 0600); e != nil {
			close(done)
			wg.Wait()
			t.Fatal(e)
		}
	}
	close(done)
	wg.Wait()
	select {
	case e := <-errs:
		t.Fatal(e)
	default:
	}
	old := time.Unix(100000, 0)
	os.Chtimes(p, old, old)
	if e := Write(p, a, 0600); e != nil {
		t.Fatal(e)
	}
	s, _ := os.Stat(p)
	if !s.ModTime().Equal(old) {
		t.Fatal("unchanged timestamp replaced")
	}
}
func TestContainment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600)
	if Within(root, outside) || Within(root, root+"-other") {
		t.Fatal("prefix escaped")
	}
	if _, e := Resolve(root, "../secret"); e == nil {
		t.Fatal("traversal")
	}
	if e := os.Symlink(outside, filepath.Join(root, "link")); e == nil {
		if _, e = Resolve(root, "link/secret"); e == nil {
			t.Fatal("symlink escaped")
		}
	}
}
