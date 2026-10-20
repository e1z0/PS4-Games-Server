// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"fmt"
	"io/fs"
	"reflect"
)

// ChangeStamp includes the platform's change timestamp when one is available.
// This detects edits that preserve a file's size and modification time.
func ChangeStamp(info fs.FileInfo) string {
	v := reflect.ValueOf(info.Sys())
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
