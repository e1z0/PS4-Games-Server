// SPDX-License-Identifier: GPL-2.0-only
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/ps4"
	"ps4server/internal/scanner"
	"ps4server/internal/storage"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const Details = "id,name,url,summary,storyline,alternative_names.*,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,release_dates.date,release_dates.human,release_dates.platform,release_dates.release_region.region,genres.name,themes.name,game_modes.name,multiplayer_modes.*,language_supports.language.name,language_supports.language_support_type.name,rating,rating_count,aggregated_rating,aggregated_rating_count,total_rating,total_rating_count,age_ratings.organization.name,age_ratings.rating_category.rating,age_ratings.rating_content_descriptions.description,age_ratings.rating_cover_url,age_ratings.synopsis,cover.*,artworks.*,screenshots.*,videos.*,websites.type.type,websites.url,websites.trusted,franchises.name,collections.name,dlcs.name,expansions.name,standalone_expansions.name,remakes.name,remasters.name,ports.name,expanded_games.name,parent_game.name"

type Entry struct {
	Expires time.Time `json:"expires"`
	Found   bool      `json:"found"`
	Data    Object    `json:"igdb,omitempty"`
}
type Cache struct {
	Version int              `json:"version"`
	Titles  map[string]Entry `json:"titles"`
	Images  ps4.ImageCache   `json:"images,omitempty"`
}
type Service struct {
	HTTP        *http.Client
	TokenURL    string
	GamesURL    string
	ImageBase   string
	Now         func() time.Time
	token       string
	expiry      time.Time
	last        time.Time
	credentials string
}

func New() *Service {
	return &Service{HTTP: &http.Client{Timeout: 15 * time.Second}, TokenURL: "https://id.twitch.tv/oauth2/token", GamesURL: "https://api.igdb.com/v4/games", ImageBase: "https://images.igdb.com/igdb/image/upload/t_1080p/", Now: time.Now}
}
func Normalize(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}
func comparableTitle(s string) string {
	s = Normalize(s)
	for _, suffix := range []string{"game of the year edition", "game of the year", "goty edition", "goty", "complete edition", "definitive edition", "deluxe edition", "ultimate edition", "special edition"} {
		if strings.HasSuffix(s, " "+suffix) {
			return strings.TrimSpace(strings.TrimSuffix(s, suffix))
		}
	}
	return s
}
func Similarity(a, b string) float64 {
	a, b = comparableTitle(a), comparableTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	left, right := map[string]int{}, map[string]int{}
	numbers := func(tokens map[string]int) string {
		var out []string
		for token := range tokens {
			isNumber := true
			for _, r := range token {
				if !unicode.IsDigit(r) {
					isNumber = false
					break
				}
			}
			if isNumber || romanNumerals[token] {
				out = append(out, token)
			}
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for _, token := range strings.Fields(a) {
		left[token]++
	}
	for _, token := range strings.Fields(b) {
		right[token]++
	}
	// A high franchise-name overlap must not substitute another numbered sequel.
	if numbers(left) != numbers(right) {
		return 0
	}
	common := 0
	for token, count := range left {
		common += min(count, right[token])
	}
	return float64(common) / float64(len(strings.Fields(a))+len(strings.Fields(b))-common)
}

var romanNumerals = map[string]bool{"i": true, "ii": true, "iii": true, "iv": true, "v": true, "vi": true, "vii": true, "viii": true, "ix": true, "x": true, "xi": true, "xii": true, "xiii": true, "xiv": true, "xv": true, "xvi": true}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) tokenFor(ctx context.Context, c config.IGDB) error {
	credentials := c.ClientID + "\x00" + c.ClientSecret
	if s.token != "" && s.Now().Before(s.expiry) && s.credentials == credentials {
		return nil
	}
	form := url.Values{"client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "grant_type": {"client_credentials"}}
	r, e := http.NewRequestWithContext(ctx, "POST", s.TokenURL, strings.NewReader(form.Encode()))
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, e := s.HTTP.Do(r)
	if e != nil {
		return errors.New("IGDB authentication unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("IGDB authentication HTTP %d", res.StatusCode)
	}
	var b struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&b); e != nil || b.Token == "" {
		return errors.New("invalid IGDB authentication response")
	}
	s.token = b.Token
	s.credentials = credentials
	s.expiry = s.Now().Add(time.Duration(b.Expires)*time.Second - 30*time.Second)
	return nil
}
func (s *Service) request(ctx context.Context, c config.IGDB, body string) ([]Object, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if e := wait(ctx, time.Duration(c.IntervalMS)*time.Millisecond-time.Since(s.last)); e != nil {
			return nil, e
		}
		if e := s.tokenFor(ctx, c); e != nil {
			return nil, e
		}
		r, e := http.NewRequestWithContext(ctx, "POST", s.GamesURL, strings.NewReader(body))
		if e != nil {
			return nil, e
		}
		r.Header.Set("Client-ID", c.ClientID)
		r.Header.Set("Authorization", "Bearer "+s.token)
		r.Header.Set("Content-Type", "text/plain")
		s.last = time.Now()
		res, e := s.HTTP.Do(r)
		if e != nil {
			return nil, errors.New("IGDB network unavailable")
		}
		if res.StatusCode == 401 {
			res.Body.Close()
			s.token = ""
			continue
		}
		if res.StatusCode == 429 {
			delay, _ := strconv.Atoi(res.Header.Get("Retry-After"))
			res.Body.Close()
			if delay < 1 {
				delay = 1
			}
			if delay > 30 {
				delay = 30
			}
			if e = wait(ctx, time.Duration(delay)*time.Second); e != nil {
				return nil, e
			}
			continue
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return nil, fmt.Errorf("IGDB HTTP %d", res.StatusCode)
		}
		var out []Object
		decoder := json.NewDecoder(io.LimitReader(res.Body, 8<<20))
		e = decoder.Decode(&out)
		if e == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				e = errors.New("invalid IGDB response: trailing data")
			}
		}
		res.Body.Close()
		if e == nil && out == nil {
			e = errors.New("invalid IGDB response: expected array")
		}
		return out, e
	}
	return nil, errors.New("IGDB retry limit reached")
}
func (s *Service) lookup(ctx context.Context, c config.IGDB, title string) (Object, error) {
	query := Normalize(title)
	count := 0
	for index := range query {
		if count == 160 {
			query = query[:index]
			break
		}
		count++
	}
	if query == "" {
		return nil, nil
	}
	candidates, e := s.request(ctx, c, `fields id,name,alternative_names.name; search "`+query+`"; where platforms = (48); limit 10;`)
	if e != nil {
		return nil, e
	}
	var best Object
	score := 0.0
	for _, g := range candidates {
		v := Similarity(title, str(g["name"]))
		for _, x := range arr(g["alternative_names"]) {
			if a := Similarity(title, str(obj(x)["name"])); a > v {
				v = a
			}
		}
		if v > score && num(g["id"]) > 0 {
			score = v
			best = g
		}
	}
	if score < 0.8 {
		return nil, nil
	}
	games, e := s.request(ctx, c, "fields "+Details+"; where id = "+sourceID(best)+" & platforms = (48); limit 1;")
	if e != nil {
		return nil, e
	}
	if len(games) == 0 {
		return nil, nil
	}
	return MapGame(games[0]), nil
}
func LoadOverrides(dir string) (map[string]Object, error) {
	m := map[string]Object{}
	e := storage.Read(filepath.Join(dir, "metadata-overrides.json"), &m)
	if errors.Is(e, os.ErrNotExist) {
		e = nil
	}
	return m, e
}
func (s *Service) Enrich(ctx context.Context, c config.Config, entries []scanner.Cached) (map[string]Object, []string, error) {
	return s.EnrichWithProgress(ctx, c, entries, nil)
}
func (s *Service) EnrichWithProgress(ctx context.Context, c config.Config, entries []scanner.Cached, progress func(int, int, string)) (map[string]Object, []string, error) {
	return s.enrich(ctx, c, entries, progress, true)
}

// Restore only rehydrates cached details and existing local image URLs. Full
// image validation/repair belongs to scans, not to the HTTP listener startup.
func (s *Service) Restore(ctx context.Context, c config.Config, entries []scanner.Cached) (map[string]Object, []string, error) {
	c.IGDB.Enabled = false
	return s.enrich(ctx, c, entries, nil, false)
}
func (s *Service) enrich(ctx context.Context, c config.Config, entries []scanner.Cached, progress func(int, int, string), validateImages bool) (result map[string]Object, warnings []string, resultErr error) {
	cache := Cache{Version: 1, Titles: map[string]Entry{}}
	if e := storage.Read(filepath.Join(c.DataDir, "metadata-cache.json"), &cache); e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, nil, errors.New("invalid metadata cache")
	}
	if cache.Version != 1 || cache.Titles == nil {
		return nil, nil, errors.New("unsupported metadata cache")
	}
	if cache.Images == nil {
		cache.Images = ps4.ImageCache{}
	}
	// Preserve completed lookups and validation stamps even when a later request
	// is cancelled. Restore is read-only and never records unvalidated images.
	if validateImages {
		defer func() {
			if err := storage.JSON(filepath.Join(c.DataDir, "metadata-cache.json"), cache); err != nil && resultErr == nil {
				resultErr = err
			}
		}()
	}
	overrides, e := LoadOverrides(c.DataDir)
	if e != nil {
		return nil, nil, e
	}
	titles := map[string]string{}
	for _, v := range entries {
		id := v.Package.TitleID
		if id != "" {
			if _, ok := titles[id]; !ok {
				titles[id] = ""
			}
			if ps4.Classify(v.Package.Category, v.Category) == "game" {
				titles[id] = v.Package.Title
			}
		}
	}
	ids := []string{}
	for id := range titles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := map[string]Object{}
	warnings = []string{}
	pending := map[string]bool{}
	total, done := 0, 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, warnings, ctx.Err()
		}
		entry, exists := cache.Titles[id]
		work := s.needsLookup(c, entry, exists, titles[id])
		if validateImages && c.IGDB.Enabled {
			data := metadataFor(entry, overrides[id])
			for _, x := range arr(obj(data["media"])["screenshots"]) {
				rel, dest, ok := screenshotPath(c, id, str(obj(x)["image_id"]))
				if ok && !cache.Images.Valid(rel, dest) {
					work = true
				}
			}
		}
		pending[id] = work
		if work {
			total++
		}
	}
	if progress != nil && total == 0 {
		progress(0, total, "")
	}
	networkFailed := false
	for _, id := range ids {
		if progress != nil && pending[id] {
			progress(done, total, titles[id])
		}
		if ctx.Err() != nil {
			return nil, warnings, ctx.Err()
		}
		entry, exists := cache.Titles[id]
		title := titles[id]
		if s.needsLookup(c, entry, exists, title) && !networkFailed {
			m, err := s.lookup(ctx, c.IGDB, title)
			if err != nil {
				networkFailed = true
				warnings = append(warnings, "IGDB unavailable; keeping cached metadata and retrying next scan")
			} else {
				days := c.IGDB.NegativeDays
				if m != nil {
					days = c.IGDB.PositiveDays
				}
				entry = Entry{Expires: s.Now().Add(time.Duration(days) * 24 * time.Hour), Found: m != nil, Data: m}
				cache.Titles[id] = entry
				// Checkpoint before downloading screenshots or looking up another
				// title, so an interrupted initial scan does not fetch it again.
				if err := storage.JSON(filepath.Join(c.DataDir, "metadata-cache.json"), cache); err != nil {
					return nil, warnings, err
				}
			}
		}
		data := metadataFor(entry, overrides[id])
		if data != nil { // Clone before rewriting cached screenshot URLs for this listener.
			b, _ := json.Marshal(data)
			data = nil
			json.Unmarshal(b, &data)
			if err := s.screenshots(ctx, c, id, data, validateImages, cache.Images); err != nil {
				warnings = append(warnings, "screenshot repair incomplete for "+id)
			}
			out[id] = data
		}
		if pending[id] {
			done++
			if progress != nil {
				progress(done, total, "")
			}
		}
	}
	if ctx.Err() != nil {
		return nil, warnings, ctx.Err()
	}
	return out, warnings, nil
}

func (s *Service) needsLookup(c config.Config, entry Entry, exists bool, title string) bool {
	return (!exists || !s.Now().Before(entry.Expires)) && title != "" && c.IGDB.Enabled && c.IGDB.ClientID != "" && c.IGDB.ClientSecret != ""
}

func metadataFor(entry Entry, override Object) Object {
	var data Object
	if entry.Found {
		data = entry.Data
	}
	if override != nil {
		data = merge(data, override)
		if data["schema_version"] == nil {
			data["schema_version"] = 1
		}
	}
	return data
}

func screenshotPath(c config.Config, id, imageID string) (string, string, bool) {
	if !config.SafeName.MatchString(id) || !config.SafeName.MatchString(imageID) {
		return "", "", false
	}
	rel := "igdb/screenshots/" + id + "/" + imageID + ".jpg"
	return rel, filepath.Join(c.CoversDir(), filepath.FromSlash(rel)), true
}

func (s *Service) screenshots(ctx context.Context, c config.Config, id string, m Object, validate bool, images ps4.ImageCache) error {
	if !config.SafeName.MatchString(id) {
		return nil
	}
	var last error
	for _, x := range arr(obj(m["media"])["screenshots"]) {
		v := obj(x)
		imageID := str(v["image_id"])
		rel, dest, ok := screenshotPath(c, id, imageID)
		if !ok {
			continue
		}
		valid := false
		if validate {
			valid = images.Valid(rel, dest)
		} else {
			info, err := os.Stat(dest)
			valid = err == nil && info.Mode().IsRegular() && info.Size() > 0
		}
		if !valid && c.IGDB.Enabled {
			r, e := http.NewRequestWithContext(ctx, "GET", s.ImageBase+url.PathEscape(imageID)+".jpg", nil)
			if e != nil {
				return e
			}
			res, e := s.HTTP.Do(r)
			if e != nil {
				last = errors.New("screenshot network unavailable")
				continue
			}
			b, e := io.ReadAll(io.LimitReader(res.Body, (25<<20)+1))
			res.Body.Close()
			if e != nil || res.StatusCode != 200 || len(b) > 25<<20 || !ps4.ValidImage(b) {
				last = errors.New("invalid screenshot response")
				continue
			}
			if e = storage.Write(dest, b, 0600); e != nil {
				last = e
				continue
			}
			valid = images.Valid(rel, dest)
		}
		if valid {
			v["source_url"] = imageURL(imageID)
			v["url"] = c.Downloads() + "/covers/" + rel
			v["cached"] = true
		} else {
			v["cached"] = false
		}
	}
	return last
}
