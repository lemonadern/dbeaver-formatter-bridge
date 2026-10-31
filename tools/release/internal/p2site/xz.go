// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package p2site

import (
	"encoding/binary"
	"hash/crc32"
)

// encodeXZ writes the XZ stream format with CRC32 and one LZMA2 filter.
// LZMA2 uncompressed chunks avoid a compressor dependency while remaining
// readable by standard XZ decoders (including Equinox's XZ repository reader).
// See https://tukaani.org/xz/xz-file-format.txt sections 2-4 and
// https://git.tukaani.org/?p=xz.git;a=blob;f=doc/lzma-file-format.txt .
func encodeXZ(data []byte) []byte {
	out := []byte{0xfd, '7', 'z', 'X', 'Z', 0, 0, 1}
	out = crcAppend(out, out[6:8])
	header := []byte{2, 0, 0x21, 1, 0, 0, 0, 0}
	out = append(out, crcAppend(header, header)...)
	start := len(out)
	for offset := 0; offset < len(data); {
		n := min(65536, len(data)-offset)
		control := byte(2)
		if offset == 0 {
			control = 1
		}
		out = append(out, control, byte((n-1)>>8), byte(n-1))
		out = append(out, data[offset:offset+n]...)
		offset += n
	}
	out = append(out, 0)
	compressed := len(out) - start
	for (len(out)-start)%4 != 0 {
		out = append(out, 0)
	}
	out = crcAppend(out, data)
	index := []byte{0, 1}
	index = appendVLI(index, uint64(12+compressed+4))
	index = appendVLI(index, uint64(len(data)))
	for len(index)%4 != 0 {
		index = append(index, 0)
	}
	index = crcAppend(index, index)
	out = append(out, index...)
	footer := make([]byte, 6)
	binary.LittleEndian.PutUint32(footer, uint32(len(index)/4-1))
	footer[5] = 1
	out = crcAppend(out, footer)
	out = append(out, footer...)
	return append(out, 'Y', 'Z')
}
func crcAppend(dst, data []byte) []byte {
	return binary.LittleEndian.AppendUint32(dst, crc32.ChecksumIEEE(data))
}
func appendVLI(dst []byte, value uint64) []byte {
	for value >= 128 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}
