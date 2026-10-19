// SPDX-License-Identifier: GPL-2.0-only
package metadata

import (
	"fmt"
	"net/url"
	"time"
)

type Object = map[string]any

func obj(v any) Object {
	m, _ := v.(map[string]any)
	if m == nil {
		return Object{}
	}
	return m
}
func arr(v any) []any {
	a, _ := v.([]any)
	if a == nil {
		return []any{}
	}
	return a
}
func str(v any) string  { s, _ := v.(string); return s }
func num(v any) float64 { n, _ := v.(float64); return n }
func flag(v any) bool   { b, _ := v.(bool); return b }
func names(v any) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range arr(v) {
		s := str(obj(x)["name"])
		if s != "" && !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}
func refs(v any) []any {
	out := []any{}
	for _, x := range arr(v) {
		m := obj(x)
		out = append(out, Object{"id": m["id"], "name": m["name"]})
	}
	return out
}
func imageURL(id string) string {
	return "https://images.igdb.com/igdb/image/upload/t_1080p/" + url.PathEscape(id) + ".jpg"
}
func mapImage(v any) any {
	m := obj(v)
	id := str(m["image_id"])
	if id == "" {
		return nil
	}
	return Object{"image_id": id, "url": imageURL(id), "width": m["width"], "height": m["height"]}
}
func mapImages(v any) []any {
	out := []any{}
	for _, x := range arr(v) {
		if m := mapImage(x); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// MapGame preserves the baseline schema_version 1 client metadata structure.
func MapGame(g Object) Object {
	r := Object{"schema_version": 1, "id": g["id"], "url": g["url"], "summary": g["summary"], "storyline": g["storyline"], "alternative_names": arr(g["alternative_names"]), "genres": names(g["genres"]), "themes": names(g["themes"]), "game_modes": names(g["game_modes"]), "franchises": refs(g["franchises"]), "collections": refs(g["collections"])}
	dev, pub := []string{}, []string{}
	for _, x := range arr(g["involved_companies"]) {
		m := obj(x)
		name := str(obj(m["company"])["name"])
		if flag(m["developer"]) {
			dev = append(dev, name)
		}
		if flag(m["publisher"]) {
			pub = append(pub, name)
		}
	}
	r["developers"] = dev
	r["publishers"] = pub
	dates := []any{}
	for _, x := range arr(g["release_dates"]) {
		m := obj(x)
		if num(m["platform"]) == 48 {
			dates = append(dates, Object{"date": time.Unix(int64(num(m["date"])), 0).UTC().Format("2006-01-02"), "human": m["human"], "region": obj(m["release_region"])["region"]})
		}
	}
	r["ps4_release_dates"] = dates
	mp := Object{"lan_co_op": false, "campaign_co_op": false, "drop_in": false}
	for _, scope := range []string{"offline", "online"} {
		mp[scope] = Object{"supported": false, "max_players": nil, "co_op": false, "co_op_max_players": nil, "split_screen": false}
	}
	for _, x := range arr(g["multiplayer_modes"]) {
		m := obj(x)
		if p := num(m["platform"]); p != 0 && p != 48 {
			continue
		}
		for _, scope := range []string{"offline", "online"} {
			v := obj(mp[scope])
			for k, field := range map[string]string{"max_players": scope + "max", "co_op_max_players": scope + "coopmax"} {
				if num(m[field]) > num(v[k]) {
					v[k] = m[field]
				}
			}
			v["co_op"] = flag(v["co_op"]) || flag(m[scope+"coop"])
			v["supported"] = num(v["max_players"]) > 1 || flag(v["co_op"])
			split := "splitscreen"
			if scope == "online" {
				split = "splitscreenonline"
			}
			v["split_screen"] = flag(v["split_screen"]) || flag(m[split])
		}
		for k, field := range map[string]string{"lan_co_op": "lancoop", "campaign_co_op": "campaigncoop", "drop_in": "dropin"} {
			mp[k] = flag(mp[k]) || flag(m[field])
		}
	}
	r["multiplayer"] = mp
	langs := Object{"interface": []string{}, "subtitles": []string{}, "voice": []string{}}
	for _, x := range arr(g["language_supports"]) {
		m := obj(x)
		kind := str(obj(m["language_support_type"])["name"])
		key := map[string]string{"Interface": "interface", "Subtitles": "subtitles", "Audio": "voice"}[kind]
		if key != "" {
			langs[key] = append(langs[key].([]string), str(obj(m["language"])["name"]))
		}
	}
	r["languages"] = langs
	ratings := Object{}
	for k, field := range map[string]string{"users": "rating", "critics": "aggregated_rating", "combined": "total_rating"} {
		ratings[k] = Object{"score": g[field], "count": g[field+"_count"]}
	}
	r["ratings"] = ratings
	ages := []any{}
	for _, x := range arr(g["age_ratings"]) {
		m := obj(x)
		descriptions := []string{}
		for _, d := range arr(m["rating_content_descriptions"]) {
			descriptions = append(descriptions, str(obj(d)["description"]))
		}
		ages = append(ages, Object{"organization": obj(m["organization"])["name"], "rating": obj(m["rating_category"])["rating"], "content_descriptions": descriptions, "synopsis": m["synopsis"], "cover_url": m["rating_cover_url"]})
	}
	r["age_ratings"] = ages
	r["media"] = Object{"cover": mapImage(g["cover"]), "artworks": mapImages(g["artworks"]), "screenshots": mapImages(g["screenshots"])}
	videos := []any{}
	for _, x := range arr(g["videos"]) {
		m := obj(x)
		if id := str(m["video_id"]); id != "" {
			videos = append(videos, Object{"name": m["name"], "video_id": id, "url": "https://www.youtube.com/watch?v=" + url.QueryEscape(id)})
		}
	}
	r["videos"] = videos
	websites := []any{}
	for _, x := range arr(g["websites"]) {
		m := obj(x)
		websites = append(websites, Object{"type": obj(m["type"])["type"], "url": m["url"], "trusted": m["trusted"]})
	}
	r["websites"] = websites
	relationships := Object{"parent_game": nil}
	for _, key := range []string{"dlcs", "expansions", "standalone_expansions", "remakes", "remasters", "ports", "expanded_games"} {
		relationships[key] = refs(g[key])
	}
	if m := obj(g["parent_game"]); len(m) > 0 {
		relationships["parent_game"] = Object{"id": m["id"], "name": m["name"]}
	}
	r["relationships"] = relationships
	return r
}
func merge(a, b Object) Object {
	out := Object{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if m, ok := v.(map[string]any); ok {
			out[k] = merge(obj(out[k]), m)
		} else {
			out[k] = v
		}
	}
	return out
}
func sourceID(g Object) string { return fmt.Sprintf("%.0f", num(g["id"])) }
