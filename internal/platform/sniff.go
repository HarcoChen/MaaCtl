package platform

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// headerBytes is how much of a binary is read to identify its format. It is
// enough for the PE, ELF, and Mach-O headers, including a universal binary's
// architecture table.
const headerBytes = 4096

// ErrUnknownArch reports a file whose header is not a recognizable executable
// or shared library format.
var ErrUnknownArch = errors.New("unrecognized binary format")

// Architectures reports the CPU architectures a native executable or shared
// library was built for. A universal (fat) Mach-O binary yields every
// architecture it carries, in file order.
func Architectures(path string) ([]Arch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	head := make([]byte, headerBytes)
	read, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	head = head[:read]
	switch {
	case isPE(head):
		return portables(head)
	case isELF(head):
		return elfs(head)
	case isMachO(head):
		return machos(head)
	default:
		return nil, fmt.Errorf("%s: %w", path, ErrUnknownArch)
	}
}

func isPE(head []byte) bool {
	return len(head) >= 2 && head[0] == 'M' && head[1] == 'Z'
}

func isELF(head []byte) bool {
	return len(head) >= 4 && string(head[:4]) == "\x7fELF"
}

// isMachO reports whether head starts with a Mach-O or universal binary header.
func isMachO(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	be := binary.BigEndian.Uint32(head)
	return be == 0xcafebabe || isMachOMagic(be) || isMachOMagic(binary.LittleEndian.Uint32(head))
}

// isMachOMagic reports whether value is a thin Mach-O magic number.
func isMachOMagic(value uint32) bool {
	return value == 0xfeedfacf || value == 0xfeedface
}

// portables reads the architecture out of a Windows PE header.
func portables(head []byte) ([]Arch, error) {
	if len(head) < 0x40 {
		return nil, ErrUnknownArch
	}
	// The DOS stub points at the PE signature.
	offset := int(binary.LittleEndian.Uint32(head[0x3c:]))
	if offset+6 > len(head) || string(head[offset:offset+4]) != "PE\x00\x00" {
		return nil, ErrUnknownArch
	}
	switch machine := binary.LittleEndian.Uint16(head[offset+4:]); machine {
	case 0x8664:
		return []Arch{AMD64}, nil
	case 0xaa64:
		return []Arch{AArch64}, nil
	default:
		return nil, fmt.Errorf("%w: PE machine %#x", ErrUnknownArch, machine)
	}
}

// elfs reads the architecture out of an ELF header.
func elfs(head []byte) ([]Arch, error) {
	if len(head) < 20 {
		return nil, ErrUnknownArch
	}
	var order binary.ByteOrder = binary.LittleEndian
	if head[5] == 2 {
		order = binary.BigEndian
	}
	switch machine := order.Uint16(head[18:]); machine {
	case 62:
		return []Arch{AMD64}, nil
	case 183:
		return []Arch{AArch64}, nil
	default:
		return nil, fmt.Errorf("%w: ELF machine %d", ErrUnknownArch, machine)
	}
}

// machos reads the architecture out of a Mach-O header, following the
// architecture table of a universal binary.
func machos(head []byte) ([]Arch, error) {
	magic := binary.BigEndian.Uint32(head)
	if isMachOMagic(magic) || isMachOMagic(binary.LittleEndian.Uint32(head)) {
		arch, ok := machoCPU(binary.LittleEndian.Uint32(head[4:]))
		if !ok {
			return nil, ErrUnknownArch
		}
		return []Arch{arch}, nil
	}
	// A universal binary stores a big-endian table of architectures.
	var arches []Arch
	for i, count := 0, int(binary.BigEndian.Uint32(head[4:])); i < count; i++ {
		offset := 8 + i*20
		if offset+4 > len(head) {
			break
		}
		if arch, ok := machoCPU(binary.BigEndian.Uint32(head[offset:])); ok {
			arches = append(arches, arch)
		}
	}
	if len(arches) == 0 {
		return nil, ErrUnknownArch
	}
	return arches, nil
}

// machoCPU maps a Mach-O CPU type onto a platform architecture.
func machoCPU(cpu uint32) (Arch, bool) {
	switch cpu {
	case 0x01000007, 0x00000007:
		return AMD64, true
	case 0x0100000c, 0x0000000c:
		return AArch64, true
	default:
		return "", false
	}
}
