// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package free

import (
	"bytes"
	"fmt"
	"strconv"
)

type meminfomap map[string]uint64

// meminfoFromBytes returns a mapping that represents the fields contained in a
// byte stream with a content compatible with /proc/meminfo
func meminfoFromBytes(buf []byte) (meminfomap, error) {
	ret := make(meminfomap)
	for line := range bytes.SplitSeq(buf, []byte{'\n'}) {
		kv := bytes.SplitN(line, []byte{':'}, 2)
		if len(kv) != 2 {
			continue
		}
		key := string(kv[0])
		tokens := bytes.SplitN(bytes.TrimSpace(kv[1]), []byte{' '}, 2)
		if len(tokens) > 0 {
			value, err := strconv.ParseUint(string(tokens[0]), 10, 64)
			if err != nil {
				return nil, err
			}
			ret[key] = value
		}
	}
	return ret, nil
}

// getMainMemInfo returns the physical memory information in bytes.
func getMainMemInfo(m meminfomap) (*mainMemInfo, error) {
	fields := []string{
		"MemTotal",
		"MemFree",
		"Buffers",
		"Cached",
		"Shmem",
		"SReclaimable",
		"MemAvailable",
	}
	if missingRequiredFields(m, fields) {
		return nil, fmt.Errorf("missing required fields from meminfo")
	}

	memTotal := m["MemTotal"] << KB
	memFree := m["MemFree"] << KB
	memShared := m["Shmem"] << KB
	memCached := (m["Cached"] + m["SReclaimable"]) << KB
	memBuffers := (m["Buffers"]) << KB
	memUsed := memTotal - memFree - memCached - memBuffers
	memAvailable := m["MemAvailable"] << KB

	mmi := mainMemInfo{
		Total:     memTotal,
		Used:      memUsed,
		Free:      memFree,
		Shared:    memShared,
		Cached:    memCached,
		Buffers:   memBuffers,
		Available: memAvailable,
	}
	return &mmi, nil
}

// getSwapInfo returns the swap space information in bytes.
func getSwapInfo(m meminfomap) (*swapInfo, error) {
	fields := []string{
		"SwapTotal",
		"SwapFree",
	}
	if missingRequiredFields(m, fields) {
		return nil, fmt.Errorf("missing required fields from meminfo")
	}
	swapTotal := m["SwapTotal"] << KB
	swapUsed := (m["SwapTotal"] - m["SwapFree"]) << KB
	swapFree := m["SwapFree"] << KB

	si := swapInfo{
		Total: swapTotal,
		Used:  swapUsed,
		Free:  swapFree,
	}
	return &si, nil
}

// missingRequiredFields checks if any of the specified fields are present in
// the input map.
func missingRequiredFields(m meminfomap, fields []string) bool {
	for _, f := range fields {
		if _, ok := m[f]; !ok {
			return true
		}
	}
	return false
}
