package core

import (
	"encoding/binary"
	"errors"
)

/*
pageFiles sums the page files in what NtQuerySystemInformation returns for
SystemPageFileInformation: a run of SYSTEM_PAGEFILE_INFORMATION entries, each
its NextEntryOffset, TotalSize and TotalInUse in pages, then its name. The
offset is from the entry it is in, and zero on the last (spec 094 R7).

It reads the bytes rather than casting them so that it runs, and is tested,
on every platform: the call is Windows's, the arithmetic is not. An empty
buffer is a machine without a page file. The offset is unsigned, so the walk
only goes forward; one that leaves the buffer is an error.
*/
func pageFiles(buf []byte, page uint64) (used, total uint64, err error) {
	const header = 12 // NextEntryOffset, TotalSize, TotalInUse
	for at := 0; at < len(buf); {
		if len(buf)-at < header {
			return 0, 0, errPageFiles
		}
		next := binary.LittleEndian.Uint32(buf[at:])
		size := uint64(binary.LittleEndian.Uint32(buf[at+4:]))
		inUse := uint64(binary.LittleEndian.Uint32(buf[at+8:]))
		if inUse > size {
			return 0, 0, errPageFiles
		}
		total += size * page
		used += inUse * page
		if next == 0 {
			return used, total, nil
		}
		at += int(next)
	}
	if len(buf) == 0 {
		return 0, 0, nil
	}
	return 0, 0, errPageFiles
}

var errPageFiles = errors.New("the page file information does not add up")
