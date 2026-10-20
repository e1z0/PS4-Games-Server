// SPDX-License-Identifier: GPL-2.0-only
package ps4

import (
	"os"
	"ps4server/internal/storage"
)

// ImageStamp records an image that has already passed full decoding. Cache
// keys are relative to the covers directory, so moving application data is safe.
type ImageStamp struct {
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Change   string `json:"change"`
}

type ImageCache map[string]ImageStamp

func (c ImageCache) Valid(key, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 32<<20 {
		delete(c, key)
		return false
	}
	stamp := ImageStamp{info.Size(), info.ModTime().UnixNano(), storage.ChangeStamp(info)}
	if previous, ok := c[key]; ok && previous == stamp {
		return true
	}
	delete(c, key)
	if !ValidImageFile(path) {
		return false
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(info, after) || stamp != (ImageStamp{after.Size(), after.ModTime().UnixNano(), storage.ChangeStamp(after)}) {
		return false
	}
	if c != nil {
		c[key] = stamp
	}
	return true
}
