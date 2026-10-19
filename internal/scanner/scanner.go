// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/ps4"
	"ps4server/internal/storage"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Record struct {
	Region          string         `json:"region"`
	TitleID         *string        `json:"title_id"`
	ContentID       *string        `json:"content_id"`
	Type            string         `json:"package_type"`
	PackageCategory *string        `json:"package_category"`
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Release         string         `json:"release"`
	Size            string         `json:"size"`
	Firmware        *string        `json:"min_fw"`
	CoverURL        *string        `json:"cover_url"`
	IGDB            map[string]any `json:"igdb,omitempty"`
}
type Cached struct {
	Mapping  string      `json:"mapping"`
	Root     string      `json:"root"`
	Relative string      `json:"relative"`
	Category string      `json:"category"`
	Size     int64       `json:"size"`
	Modified int64       `json:"modified"`
	Change   string      `json:"change"`
	Package  ps4.Package `json:"package"`
	Release  string      `json:"release"`
}
type State struct {
	Version int      `json:"version"`
	Entries []Cached `json:"entries"`
}
type Catalog struct {
	Data map[string]Record `json:"DATA"`
}
type Job struct {
	Running   bool      `json:"running"`
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished"`
	Visited   int       `json:"visited"`
	Inspected int       `json:"inspected"`
	Reused    int       `json:"reused"`
	Removed   int       `json:"removed"`
	Cancelled bool      `json:"cancelled"`
	Errors    []string  `json:"errors"`
}
type Enricher interface {
	Enrich(context.Context, config.Config, []Cached) (map[string]map[string]any, []string, error)
}
type Engine struct {
	mu       sync.RWMutex
	state    State
	job      Job
	cancel   context.CancelFunc
	logs     []string
	metadata map[string]map[string]any
	Enricher Enricher
}

func New(c config.Config) (*Engine, error) {
	s := State{Version: 1, Entries: []Cached{}}
	e := storage.Read(filepath.Join(c.DataDir, "scan-cache.json"), &s)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, fmt.Errorf("read scan cache: %w", e)
	}
	if s.Version != 1 {
		return nil, errors.New("unsupported scan cache version")
	}
	return &Engine{state: s, metadata: map[string]map[string]any{}}, nil
}
func (e *Engine) Log(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logs = append(e.logs, time.Now().Format(time.RFC3339)+" "+s)
	if len(e.logs) > 200 {
		e.logs = e.logs[len(e.logs)-200:]
	}
}
func (e *Engine) Logs() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]string{}, e.logs...)
}
func (e *Engine) Status() Job {
	e.mu.RLock()
	defer e.mu.RUnlock()
	j := e.job
	j.Errors = append([]string{}, j.Errors...)
	return j
}
func (e *Engine) Snapshot() []Cached {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]Cached{}, e.state.Entries...)
}
func (e *Engine) Cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
	}
}
func (e *Engine) begin(ctx context.Context) (context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.job.Running {
		return nil, errors.New("scan already running")
	}
	ctx, e.cancel = context.WithCancel(ctx)
	e.job = Job{Running: true, Started: time.Now(), Errors: []string{}}
	return ctx, nil
}
func (e *Engine) Start(ctx context.Context, c config.Config, force bool) error {
	ctx, err := e.begin(ctx)
	if err != nil {
		return err
	}
	go e.run(ctx, c, force)
	return nil
}
func (e *Engine) Run(ctx context.Context, c config.Config, force bool) error {
	ctx, err := e.begin(ctx)
	if err != nil {
		return err
	}
	return e.run(ctx, c, force)
}
func (e *Engine) issue(s string) {
	e.mu.Lock()
	e.job.Errors = append(e.job.Errors, s)
	e.mu.Unlock()
	e.Log(s)
}
func stamp(s fs.FileInfo) string {
	v := reflect.ValueOf(s.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		for _, key := range []string{"Ctim", "Ctimespec", "ChangeTime"} {
			f := v.FieldByName(key)
			if f.IsValid() && f.CanInterface() {
				return fmt.Sprint(f.Interface())
			}
		}
	}
	return ""
}
func same(v Cached, s fs.FileInfo) bool {
	return v.Size == s.Size() && v.Modified == s.ModTime().UnixNano() && v.Change == stamp(s)
}
func key(m, rel string) string { return m + "/" + rel }
func (e *Engine) run(ctx context.Context, c config.Config, force bool) (result error) {
	defer func() {
		e.mu.Lock()
		e.job.Running = false
		e.job.Finished = time.Now()
		e.job.Cancelled = ctx.Err() != nil
		if e.cancel != nil {
			e.cancel()
		}
		e.cancel = nil
		e.mu.Unlock()
		e.Log("scan finished")
	}()
	e.Log("scan started")
	old := e.Snapshot()
	oldMap := map[string]Cached{}
	for _, v := range old {
		oldMap[key(v.Mapping, v.Relative)] = v
	}
	next := []Cached{}
	seen := map[string]bool{}
	mappings := append([]config.Mapping{}, c.Mappings...)
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].ID < mappings[j].ID })
	for _, m := range mappings {
		if !m.Enabled {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		root, err := filepath.EvalSymlinks(m.Path)
		if err == nil {
			root, err = filepath.Abs(root)
		}
		var paths []string
		if err == nil {
			err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(p), ".pkg") {
					paths = append(paths, p)
				}
				return nil
			})
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.issue("mapping " + m.ID + " unavailable; retaining previous entries")
			for _, v := range old {
				if v.Mapping == m.ID && v.Root == m.Path {
					next = append(next, v)
				}
			}
			continue
		}
		for _, path := range paths {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			canonical, err := storage.Resolve(root, rel)
			if err != nil {
				e.issue("mapping " + m.ID + ": excluded unsafe path " + rel)
				continue
			}
			if seen[canonical] {
				continue
			}
			seen[canonical] = true
			s, err := os.Stat(canonical)
			if err != nil || !s.Mode().IsRegular() {
				e.issue("file unavailable: " + rel)
				continue
			}
			prev, exists := oldMap[key(m.ID, rel)]
			reuse := exists && prev.Root == m.Path && same(prev, s) && !force
			v := Cached{Mapping: m.ID, Root: m.Path, Relative: rel, Category: m.Category, Size: s.Size(), Modified: s.ModTime().UnixNano(), Change: stamp(s), Release: s.ModTime().Format("01-02-2006")}
			if reuse {
				v.Package = prev.Package
				v.Release = prev.Release
			} else {
				v.Package, err = ps4.Inspect(canonical)
			}
			e.mu.Lock()
			e.job.Visited++
			if reuse {
				e.job.Reused++
			} else {
				e.job.Inspected++
			}
			e.mu.Unlock()
			if err != nil {
				e.issue("invalid PS4 package: " + rel)
				continue
			}
			after, err := os.Stat(canonical)
			if err != nil || !same(v, after) {
				e.issue("file changed during scan: " + rel)
				if exists {
					next = append(next, prev)
				}
				continue
			}
			for _, warning := range v.Package.Warnings {
				e.issue(rel + ": " + warning)
			}
			if v.Package.Title == "" {
				v.Package.Title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
			}
			e.cover(c, v, canonical)
			next = append(next, v)
		}
	}
	// Deduplicate retained entries too, without discarding inaccessible mapping records.
	sort.Slice(next, func(i, j int) bool {
		return key(next[i].Mapping, next[i].Relative) < key(next[j].Mapping, next[j].Relative)
	})
	meta := map[string]map[string]any{}
	if e.Enricher != nil {
		var warnings []string
		var err error
		meta, warnings, err = e.Enricher.Enrich(ctx, c, next)
		for _, s := range warnings {
			e.issue(s)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.issue("metadata unavailable; scan remains usable offline")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := writeCatalogs(c, next, old, meta); err != nil {
		e.issue("catalog write failed")
		return err
	}
	state := State{Version: 1, Entries: next}
	if err := storage.JSON(filepath.Join(c.DataDir, "scan-cache.json"), state); err != nil {
		return err
	}
	remaining := map[string]bool{}
	for _, v := range next {
		remaining[key(v.Mapping, v.Relative)] = true
	}
	removed := 0
	for k := range oldMap {
		if !remaining[k] {
			removed++
		}
	}
	e.mu.Lock()
	e.state = state
	e.metadata = meta
	e.job.Removed = removed
	e.mu.Unlock()
	return nil
}
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func Escape(s string) string {
	parts := strings.Split(filepath.ToSlash(s), "/")
	for i, v := range parts {
		parts[i] = url.PathEscape(v)
	}
	return strings.Join(parts, "/")
}
func PackageURL(c config.Config, v Cached) string {
	return c.Downloads() + "/pkg/" + v.Mapping + "/" + Escape(v.Relative)
}
func CoverKey(v Cached) string {
	k := v.Package.TitleID
	if ps4.Classify(v.Package.Category, v.Category) == "dlc" {
		k = v.Package.ContentID
	}
	if !config.SafeName.MatchString(k) {
		h := sha256.Sum256([]byte(v.Package.ContentID + "|" + v.Mapping + "|" + v.Relative))
		k = hex.EncodeToString(h[:16])
	}
	return k
}
func coverRelative(v Cached) string { return v.Category + "/" + CoverKey(v) }
func coverPath(c config.Config, v Cached) string {
	base := filepath.Join(c.CoversDir(), filepath.FromSlash(coverRelative(v)))
	for _, ext := range []string{".png", ".jpg", ".jpeg"} {
		p := base + ext
		if ps4.ValidImageFile(p) {
			return p
		}
	}
	return base + ".png"
}
func (e *Engine) cover(c config.Config, v Cached, pkg string) {
	status, err := ps4.ExtractIcon(pkg, v.Package.Icon, coverPath(c, v))
	if err != nil {
		e.issue("cover repair failed: " + v.Relative)
	} else if status == "invalid" {
		e.issue("invalid icon: " + v.Relative)
	}
}
func RecordFor(c config.Config, v Cached, meta map[string]map[string]any) Record {
	cover := c.Downloads() + "/covers/fallback.svg"
	if p := coverPath(c, v); ps4.ValidImageFile(p) {
		rel, _ := filepath.Rel(c.CoversDir(), p)
		cover = c.Downloads() + "/covers/" + Escape(rel)
	}
	return Record{ps4.Region(v.Package.ContentID), ptr(v.Package.TitleID), ptr(v.Package.ContentID), ps4.Classify(v.Package.Category, v.Category), ptr(v.Package.Category), v.Package.Title, v.Package.Version, v.Release, strconv.FormatInt(v.Size, 10), v.Package.Firmware, &cover, meta[v.Package.TitleID]}
}
func Catalogs(c config.Config, entries []Cached, meta map[string]map[string]any) map[string]Catalog {
	out := map[string]Catalog{}
	for _, m := range c.Mappings {
		if m.Enabled {
			out[m.Category] = Catalog{map[string]Record{}}
		}
	}
	for _, v := range entries {
		cat, ok := out[v.Category]
		if !ok {
			cat = Catalog{map[string]Record{}}
		}
		cat.Data[PackageURL(c, v)] = RecordFor(c, v, meta)
		out[v.Category] = cat
	}
	return out
}
func (e *Engine) Catalogs(c config.Config) map[string]Catalog {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return Catalogs(c, e.state.Entries, e.metadata)
}
func writeCatalogs(c config.Config, next, old []Cached, meta map[string]map[string]any) error {
	cats := Catalogs(c, next, meta)
	for _, v := range old {
		if _, ok := cats[v.Category]; !ok {
			cats[v.Category] = Catalog{map[string]Record{}}
		}
	}
	for cat, data := range cats {
		if !config.SafeName.MatchString(cat) {
			return errors.New("unsafe catalog name")
		}
		if err := storage.JSON(filepath.Join(c.DataDir, "catalogs", cat+".json"), data); err != nil {
			return err
		}
	}
	return nil
}

// MarshalSnapshot lets callers retain an independent immutable view for diagnostics.
func (e *Engine) MarshalSnapshot() []byte { b, _ := json.Marshal(e.Snapshot()); return b }

// RestoreMetadata makes cached rich details available even with startup scans disabled.
func (e *Engine) RestoreMetadata(ctx context.Context, c config.Config) {
	if e.Enricher == nil {
		return
	}
	c.IGDB.Enabled = false
	m, _, err := e.Enricher.Enrich(ctx, c, e.Snapshot())
	if err != nil {
		e.Log("cached metadata unavailable")
		return
	}
	e.mu.Lock()
	e.metadata = m
	e.mu.Unlock()
}
