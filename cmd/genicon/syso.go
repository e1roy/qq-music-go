package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// iconSyso makes a COFF object (.syso) with the images of an .ico file as
// icon resources and their group as icon 1, which the window class of MyGo
// and Explorer use. The Go linker links the .syso files of the main
// package into the executable. Adapted from cmd/mygo/winres.go of MyGo
// (MIT), without the manifest and version resources: MyGo reads those to
// tell an app built by mygo build.
func iconSyso(ico []byte, goarch string) ([]byte, error) {
	arch, ok := map[string]struct{ machine, relocation uint16 }{
		"amd64": {0x8664, 0x0003}, // IMAGE_REL_AMD64_ADDR32NB
		"arm64": {0xAA64, 0x0002}, // IMAGE_REL_ARM64_ADDR32NB
		"386":   {0x014C, 0x0007}, // IMAGE_REL_I386_DIR32NB
	}[goarch]
	if !ok {
		return nil, fmt.Errorf("no Windows resources for %s", goarch)
	}
	const rtIcon, rtGroupIcon = 3, 14
	le := binary.LittleEndian

	// An .ico is a directory of 16-byte entries, each with the offset of its
	// image; a group resource has 14-byte entries, each with an icon's id.
	if len(ico) < 6 || le.Uint16(ico[2:]) != 1 {
		return nil, errors.New("icon.ico: not an icon")
	}
	n := int(le.Uint16(ico[4:]))
	res := map[uint16]map[uint16][]byte{rtIcon: {}}
	var group bytes.Buffer
	_ = binary.Write(&group, le, [3]uint16{0, 1, uint16(n)})
	for i := range n {
		e := ico[6+16*i:]
		if len(e) < 16 {
			return nil, errors.New("icon.ico: truncated")
		}
		size, at := le.Uint32(e[8:]), le.Uint32(e[12:])
		if int(at)+int(size) > len(ico) {
			return nil, errors.New("icon.ico: truncated")
		}
		id := uint16(i + 1)
		res[rtIcon][id] = ico[at : at+size]
		group.Write(e[:12])
		_ = binary.Write(&group, le, id)
	}
	res[rtGroupIcon] = map[uint16][]byte{1: group.Bytes()}

	// The resource section: three levels of directories (type, id,
	// language), the data entries, then the data. Directory entries point
	// to subdirectories by section offset; data entries hold relative
	// virtual addresses, which the linker computes from relocations.
	types := sortedKeys(res)
	dirSize := func(n int) int { return 16 + 8*n }
	size := dirSize(len(types))
	count := 0
	for _, t := range types {
		size += dirSize(len(res[t])) + len(res[t])*dirSize(1)
		count += len(res[t])
	}
	entriesAt := size
	dataAt := entriesAt + 16*count

	raw := make([]byte, dataAt)
	dir := func(at, n int) { le.PutUint16(raw[at+14:], uint16(n)) } // NumberOfIdEntries
	var relocs []uint32
	var data bytes.Buffer
	dir(0, len(types))
	next := dirSize(len(types))
	entry := entriesAt
	for i, t := range types {
		le.PutUint32(raw[16+8*i:], uint32(t))
		le.PutUint32(raw[16+8*i+4:], uint32(next)|0x80000000)
		ids := sortedKeys(res[t])
		idDir := next
		dir(idDir, len(ids))
		next += dirSize(len(ids))
		for j, id := range ids {
			le.PutUint32(raw[idDir+16+8*j:], uint32(id))
			le.PutUint32(raw[idDir+16+8*j+4:], uint32(next)|0x80000000)
			dir(next, 1)
			le.PutUint32(raw[next+16:], 0) // LANG_NEUTRAL
			le.PutUint32(raw[next+20:], uint32(entry))
			next += dirSize(1)

			for data.Len()%8 != 0 {
				data.WriteByte(0)
			}
			le.PutUint32(raw[entry:], uint32(dataAt+data.Len())) // OffsetToData
			le.PutUint32(raw[entry+4:], uint32(len(res[t][id])))
			relocs = append(relocs, uint32(entry))
			entry += 16
			data.Write(res[t][id])
		}
	}
	raw = append(raw, data.Bytes()...)
	for len(raw)%4 != 0 {
		raw = append(raw, 0)
	}

	var out bytes.Buffer
	const headerSize, sectionHeaderSize = 20, 40
	rawAt := headerSize + sectionHeaderSize
	relocAt := rawAt + len(raw)
	symbolsAt := relocAt + 10*len(relocs)
	_ = binary.Write(&out, le, struct {
		Machine, Sections                   uint16
		Time, Symbols, SymbolCount          uint32
		OptionalHeaderSize, Characteristics uint16
	}{arch.machine, 1, 0, uint32(symbolsAt), 1, 0, 0})
	_ = binary.Write(&out, le, struct {
		Name                                        [8]byte
		VirtualSize, VirtualAddress, RawSize, RawAt uint32
		RelocAt, LineNumbersAt                      uint32
		Relocs, LineNumbers                         uint16
		Characteristics                             uint32
	}{[8]byte{'.', 'r', 's', 'r', 'c'}, 0, 0, uint32(len(raw)), uint32(rawAt), uint32(relocAt), 0, uint16(len(relocs)), 0, 0x40000040})
	out.Write(raw)
	for _, r := range relocs {
		_ = binary.Write(&out, le, r)         // VirtualAddress
		_ = binary.Write(&out, le, uint32(0)) // symbol 0: the section
		_ = binary.Write(&out, le, arch.relocation)
	}
	// The section symbol, then an empty string table.
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	_ = binary.Write(&out, le, uint32(0)) // Value
	_ = binary.Write(&out, le, uint16(1)) // SectionNumber
	_ = binary.Write(&out, le, uint16(0)) // Type
	out.Write([]byte{3, 0})               // IMAGE_SYM_CLASS_STATIC, no aux symbols
	_ = binary.Write(&out, le, uint32(4))
	return out.Bytes(), nil
}

func sortedKeys[V any](m map[uint16]V) []uint16 {
	keys := make([]uint16, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
