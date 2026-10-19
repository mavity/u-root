// Copyright 2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build tamago || unix || wasip1

package ls

import (
	"fmt"
	"os/user"
	"sync"
)

// Without this cache, `ls -l` is orders of magnitude slower.
var (
	cacheMu  sync.RWMutex
	uidCache = map[uint32]string{}
	gidCache = map[uint32]string{}
)

// Convert uid to username, or return uid on error.
func lookupUserName(id uint32) string {
	cacheMu.RLock()
	s, ok := uidCache[id]
	cacheMu.RUnlock()
	if ok {
		return s
	}
	s = fmt.Sprint(id)
	if u, err := user.LookupId(s); err == nil {
		s = u.Username
	}
	cacheMu.Lock()
	uidCache[id] = s
	cacheMu.Unlock()
	return s
}

// Convert gid to group name, or return gid on error.
func lookupGroupName(id uint32) string {
	cacheMu.RLock()
	s, ok := gidCache[id]
	cacheMu.RUnlock()
	if ok {
		return s
	}
	s = fmt.Sprint(id)
	if g, err := user.LookupGroupId(s); err == nil {
		s = g.Name
	}
	cacheMu.Lock()
	gidCache[id] = s
	cacheMu.Unlock()
	return s
}
