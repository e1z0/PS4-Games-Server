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
	// Canonical is used only to deduplicate retained unavailable mappings; reuse keys stay mapping-relative.
	Canonical string      `json:"canonical,omitempty"`
	Mapping   string      `json:"mapping"`
	Root      string      `json:"root"`
	Relative  string      `json:"relative"`
	Category  string      `json:"category"`
	Size      int64       `json:"size"`
	Modified  int64       `json:"modified"`
	Change    string      `json:"change"`
	Package   ps4.Package `json:"package"`
	Release   string      `json:"release"`
}
type State struct {
	Version int      `json:"version"`
	Entries []Cached `json:"entries"`
}
type Catalog struct {
	Data map[string]Record `json:"DATA"`
}
type Job struct {
	Phase         string    `json:"phase"`
	Total         int       `json:"total"`
	Current       string    `json:"current,omitempty"`
	MetadataDone  int       `json:"metadata_done"`
	MetadataTotal int       `json:"metadata_total"`
	Running       bool      `json:"running"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished"`
	Visited       int       `json:"visited"`
	Inspected     int       `json:"inspected"`
	Reused        int       `json:"reused"`
	Removed       int       `json:"removed"`
	Cancelled     bool      `json:"cancelled"`
	Errors        []string  `json:"errors"`
}
type Enricher interface {
	Enrich(context.Context, config.Config, []Cached) (map[string]map[string]any, []string, error)
}
type ProgressEnricher interface {
	EnrichWithProgress(context.Context, config.Config, []Cached, func(int, int, string)) (map[string]map[string]any, []string, error)
}
type Engine struct {
	mu              sync.RWMutex
	catalogMu       sync.Mutex
	catalogKey      string
	catalogRevision uint64
	published       map[string]Catalog
	revision        uint64
	state           State
	retired         []Cached
	job             Job
	cancel          context.CancelFunc
	logs            []string
	metadata        map[string]map[string]any
	Enricher        Enricher
	inspect         func(string) (ps4.Package, error)
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
	// A restarted process must never serve a cached root that the current
	// configuration no longer enables. Live changes publish with the next scan.
	allowed := map[string]string{}
	for _, m := range c.Mappings {
		if m.Enabled {
			allowed[m.ID] = m.Path
		}
	}
	entries := s.Entries[:0]
	var retired []Cached
	for _, entry := range s.Entries {
		if root, ok := allowed[entry.Mapping]; ok && root == entry.Root {
			entries = append(entries, entry)
		} else {
			retired = append(retired, entry)
		}
	}
	s.Entries = entries
	return &Engine{state: s, retired: retired, metadata: map[string]map[string]any{}, inspect: ps4.Inspect}, nil
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
	e.job = Job{Running: true, Phase: "discovering", Started: time.Now(), Errors: []string{}}
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
func same(v Cached, s fs.FileInfo) bool {
	return v.Size == s.Size() && v.Modified == s.ModTime().UnixNano() && v.Change == storage.ChangeStamp(s)
}
func key(m, rel string) string { return m + "/" + rel }
func (e *Engine) run(ctx context.Context, c config.Config, force bool) (result error) {
	defer func() {
		if result != nil && ctx.Err() == nil {
			e.issue("scan failed: " + result.Error())
		}
		e.mu.Lock()
		e.job.Running = false
		e.job.Finished = time.Now()
		e.job.Cancelled = ctx.Err() != nil
		e.job.Current = ""
		switch {
		case e.job.Cancelled:
			e.job.Phase = "cancelled"
		case result != nil:
			e.job.Phase = "failed"
		default:
			e.job.Phase = "complete"
		}
		if e.cancel != nil {
			e.cancel()
		}
		e.cancel = nil
		e.mu.Unlock()
		e.Log("scan finished")
	}()
	e.Log("scan started")
	old := e.Snapshot()
	e.mu.RLock()
	old = append(old, e.retired...)
	e.mu.RUnlock()
	oldMap := map[string]Cached{}
	for _, v := range old {
		oldMap[key(v.Mapping, v.Relative)] = v
	}
	next := []Cached{}
	retained := map[string]bool{}
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
		if err == nil {
			var info fs.FileInfo
			info, err = os.Stat(root)
			if err == nil && !info.IsDir() {
				err = fs.ErrInvalid
			}
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
			e.issue(fmt.Sprintf("Folder %s (%s) is unavailable: %v. Keeping its previous packages.", m.ID, m.Path, err))
			for _, v := range old {
				if v.Mapping == m.ID && v.Root == m.Path {
					next = append(next, v)
					retained[key(v.Mapping, v.Relative)] = true
				}
			}
			continue
		}
		e.mu.Lock()
		e.job.Total += len(paths)
		e.job.Phase = "reading"
		e.mu.Unlock()
		for _, path := range paths {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			e.mu.Lock()
			e.job.Current = rel
			e.mu.Unlock()
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
			v := Cached{Canonical: canonical, Mapping: m.ID, Root: m.Path, Relative: rel, Category: m.Category, Size: s.Size(), Modified: s.ModTime().UnixNano(), Change: storage.ChangeStamp(s), Release: s.ModTime().Format("01-02-2006")}
			if reuse {
				v.Package = prev.Package
				v.Release = prev.Release
			} else {
				v.Package, err = e.inspect(canonical)
			}
			e.mu.Lock()
			e.job.Visited++
			if reuse {
				e.job.Reused++
			} else {
				e.job.Inspected++
			}
			e.mu.Unlock()
			after, statErr := os.Stat(canonical)
			if statErr != nil || !same(v, after) {
				e.issue("file changed during scan: " + rel)
				if exists && prev.Root == m.Path {
					next = append(next, prev)
					retained[key(prev.Mapping, prev.Relative)] = true
				}
				continue
			}
			if err != nil {
				e.issue(fmt.Sprintf("Cannot read PS4 package %s: %v", rel, err))
				continue
			}
			for _, warning := range v.Package.Warnings {
				if reuse && strings.HasSuffix(warning, " additional encrypted CNT entries") {
					continue
				}
				if strings.HasSuffix(warning, " additional encrypted CNT entries") {
					// Encrypted payload sections are routine in otherwise readable
					// PS4 packages. Report them without flagging a broken scan.
					e.Log(rel + ": " + warning)
					continue
				}
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
		ki, kj := key(next[i].Mapping, next[i].Relative), key(next[j].Mapping, next[j].Relative)
		if retained[ki] != retained[kj] {
			return retained[ki]
		}
		return key(next[i].Mapping, next[i].Relative) < key(next[j].Mapping, next[j].Relative)
	})
	unique := next[:0]
	canonicalSeen := map[string]bool{}
	for _, v := range next {
		canonical := v.Canonical
		if canonical == "" {
			canonical, _ = storage.Resolve(v.Root, v.Relative)
		}
		if canonical != "" && canonicalSeen[canonical] {
			continue
		}
		if canonical != "" {
			canonicalSeen[canonical] = true
		}
		unique = append(unique, v)
	}
	next = unique
	sort.Slice(next, func(i, j int) bool {
		return key(next[i].Mapping, next[i].Relative) < key(next[j].Mapping, next[j].Relative)
	})

	// Publish native package details before optional online lookups. A slow IGDB
	// account must not keep the first usable library hidden.
	e.mu.RLock()
	meta := e.metadata
	e.mu.RUnlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	e.mu.Lock()
	e.job.Phase = "publishing"
	e.job.Current = ""
	e.mu.Unlock()
	if err := e.publish(c, next, old, meta); err != nil {
		return err
	}
	e.Log(fmt.Sprintf("%d packages available in the library (%d read, %d unchanged)", len(next), e.Status().Inspected, e.Status().Reused))
	if e.Enricher == nil {
		return nil
	}
	e.mu.Lock()
	e.job.Phase = "metadata"
	e.mu.Unlock()
	var enriched map[string]map[string]any
	var warnings []string
	var err error
	if progress, ok := e.Enricher.(ProgressEnricher); ok {
		enriched, warnings, err = progress.EnrichWithProgress(ctx, c, next, func(done, total int, current string) {
			e.mu.Lock()
			e.job.MetadataDone, e.job.MetadataTotal, e.job.Current = done, total, current
			e.mu.Unlock()
		})
	} else {
		enriched, warnings, err = e.Enricher.Enrich(ctx, c, next)
	}
	for _, warning := range warnings {
		e.issue(warning)
	}
	if _, ok := e.Enricher.(ProgressEnricher); ok && err == nil && ctx.Err() == nil {
		if total := e.Status().MetadataTotal; total == 0 {
			e.Log("Game details and screenshots reused from cache; no updates needed")
		} else {
			e.Log(fmt.Sprintf("Game details: %d titles needed updates or screenshot repair; other titles reused", total))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		e.issue("Game details could not be refreshed; local packages remain available: " + err.Error())
	}
	if enriched != nil {
		meta = enriched
	}
	e.mu.Lock()
	e.job.Phase = "publishing"
	e.job.Current = ""
	e.mu.Unlock()
	return e.publish(c, next, old, meta)
}

func (e *Engine) publish(c config.Config, next, old []Cached, meta map[string]map[string]any) error {
	if err := writeCatalogs(c, next, old, meta); err != nil {
		e.issue("Cannot save the catalog; check data-directory permissions and free space")
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
	for _, v := range old {
		if !remaining[key(v.Mapping, v.Relative)] {
			removed++
		}
	}
	e.mu.Lock()
	e.state = state
	e.retired = nil
	e.metadata = meta
	e.revision++
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
func ownCover(c config.Config, v Cached) string {
	base := filepath.Join(c.CoversDir(), filepath.FromSlash(coverRelative(v)))
	for _, ext := range []string{".png", ".jpg", ".jpeg"} {
		p := base + ext
		if ps4.ValidImageFile(p) {
			return p
		}
	}
	return base + ".png"
}

// Covers can be shared by title across categories; DLC first uses its distinct content ID.
func coverPath(c config.Config, v Cached) string {
	own := ownCover(c, v)
	if ps4.ValidImageFile(own) {
		return own
	}
	categories := []string{"games", "apps", "updates", "DLC", "demos", "homebrew", "emulators", "themes", "PS1", "PS2", "PSP"}
	for _, m := range c.Mappings {
		categories = append(categories, m.Category)
	}
	sort.Strings(categories)
	keys := []string{CoverKey(v)}
	if ps4.Classify(v.Package.Category, v.Category) == "dlc" && config.SafeName.MatchString(v.Package.TitleID) {
		keys = append(keys, v.Package.TitleID)
	}
	for _, key := range keys {
		for _, cat := range categories {
			if !config.SafeName.MatchString(cat) {
				continue
			}
			for _, ext := range []string{".png", ".jpg", ".jpeg"} {
				p := filepath.Join(c.CoversDir(), cat, key+ext)
				if ps4.ValidImageFile(p) {
					return p
				}
			}
		}
	}
	return own
}
func (e *Engine) cover(c config.Config, v Cached, pkg string) {
	status, err := ps4.ExtractIcon(pkg, v.Package.Icon, ownCover(c, v))
	if err != nil {
		e.issue("cover repair failed: " + v.Relative)
	} else if status == "invalid" {
		e.issue("invalid icon: " + v.Relative)
	}
}
func RecordFor(c config.Config, v Cached, meta map[string]map[string]any) Record {
	cover := c.Downloads() + "/covers/fallback.png"
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
	// Cover validation belongs to publishing a snapshot, not every browser/feed
	// request. Keep expensive filesystem/image work outside the job status lock.
	encoded, _ := json.Marshal(struct {
		Public, Downloads, Covers string
		Mappings                  []config.Mapping
	}{c.PublicURL, c.Downloads(), c.CoversDir(), c.Mappings})
	cacheKey := string(encoded)
	e.catalogMu.Lock()
	defer e.catalogMu.Unlock()
	e.mu.RLock()
	revision := e.revision
	if e.published != nil && e.catalogKey == cacheKey && e.catalogRevision == revision {
		e.mu.RUnlock()
		return e.published
	}
	entries := append([]Cached{}, e.state.Entries...)
	meta := e.metadata
	e.mu.RUnlock()
	cats := Catalogs(c, entries, meta)
	e.published = cats
	e.catalogKey = cacheKey
	e.catalogRevision = revision
	return cats
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
	var m map[string]map[string]any
	var err error
	if cached, ok := e.Enricher.(interface {
		Restore(context.Context, config.Config, []Cached) (map[string]map[string]any, []string, error)
	}); ok {
		m, _, err = cached.Restore(ctx, c, e.Snapshot())
	} else {
		m, _, err = e.Enricher.Enrich(ctx, c, e.Snapshot())
	}
	if err != nil {
		e.Log("cached metadata unavailable")
		return
	}
	e.mu.Lock()
	e.metadata = m
	e.revision++
	e.mu.Unlock()
}
