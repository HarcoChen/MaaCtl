package tests

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"maactl/internal/platform"
)

func TestPlatformIDsRoundTrip(t *testing.T) {
	for _, target := range platform.Supported {
		parsed, err := platform.Parse(target.ID())
		if err != nil {
			t.Fatalf("Parse(%q): %v", target.ID(), err)
		}
		if parsed != target {
			t.Errorf("Parse(%q) = %#v, want %#v", target.ID(), parsed, target)
		}
		if !target.Known() {
			t.Errorf("%s is listed as supported but Known() is false", target.ID())
		}
		if target.GoOS() == "" || target.GoARCH() == "" {
			t.Errorf("%s does not map onto GOOS/GOARCH", target.ID())
		}
	}
	if _, err := platform.Parse("win-riscv64"); err == nil {
		t.Error("expected an unknown platform to fail")
	}
}

func TestPlatformNamesLibrariesAndExecutables(t *testing.T) {
	cases := map[string]struct {
		target    platform.Target
		framework string
		toolkit   string
		exe       string
	}{
		"windows": {platform.Target{OS: platform.Windows, Arch: platform.AMD64}, "MaaFramework.dll", "MaaToolkit.dll", "maactl.exe"},
		"linux":   {platform.Target{OS: platform.Linux, Arch: platform.AArch64}, "libMaaFramework.so", "libMaaToolkit.so", "maactl"},
		"macos":   {platform.Target{OS: platform.MacOS, Arch: platform.AMD64}, "libMaaFramework.dylib", "libMaaToolkit.dylib", "maactl"},
	}
	for name, tc := range cases {
		if got := tc.target.FrameworkLibrary(); got != tc.framework {
			t.Errorf("%s framework library = %q, want %q", name, got, tc.framework)
		}
		if got := tc.target.ToolkitLibrary(); got != tc.toolkit {
			t.Errorf("%s toolkit library = %q, want %q", name, got, tc.toolkit)
		}
		if got := tc.target.Executable("maactl"); got != tc.exe {
			t.Errorf("%s executable = %q, want %q", name, got, tc.exe)
		}
	}
}

func TestPlatformHostMatchesTheBuild(t *testing.T) {
	host := platform.Host()
	if host.GoOS() != runtime.GOOS || host.GoARCH() != runtime.GOARCH {
		t.Fatalf("Host() = %s, want %s/%s", host.ID(), runtime.GOOS, runtime.GOARCH)
	}
	if !host.Known() {
		t.Fatalf("Host() = %s is not a supported platform", host.ID())
	}
	if got, want := host.Archive("maactl", "0.1.0"), "maactl-0.1.0-"+host.ID()+".zip"; got != want {
		t.Errorf("Archive = %q, want %q", got, want)
	}
}

func TestArchitecturesReadsEveryBinaryFormat(t *testing.T) {
	cases := map[string]struct {
		head []byte
		want []platform.Arch
	}{
		"pe amd64":             {fakePE(0x8664), []platform.Arch{platform.AMD64}},
		"pe arm64":             {fakePE(0xaa64), []platform.Arch{platform.AArch64}},
		"elf amd64":            {fakeELF(binary.LittleEndian, 62), []platform.Arch{platform.AMD64}},
		"elf arm64":            {fakeELF(binary.LittleEndian, 183), []platform.Arch{platform.AArch64}},
		"elf big endian arm64": {fakeELF(binary.BigEndian, 183), []platform.Arch{platform.AArch64}},
		"macho amd64":          {fakeMachO(0x01000007), []platform.Arch{platform.AMD64}},
		"macho arm64":          {fakeMachO(0x0100000c), []platform.Arch{platform.AArch64}},
		"macho universal": {
			fakeFatMachO(0x01000007, 0x0100000c),
			[]platform.Arch{platform.AMD64, platform.AArch64},
		},
	}
	for name, tc := range cases {
		path := filepath.Join(t.TempDir(), "library")
		if err := os.WriteFile(path, tc.head, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := platform.Architectures(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", name, got, tc.want)
			}
		}
	}
}

func TestArchitecturesRejectsUnknownFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library")
	if err := os.WriteFile(path, []byte("just some text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := platform.Architectures(path); err == nil {
		t.Error("expected an unknown format to fail")
	}
	if _, err := platform.Architectures(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected a missing file to fail")
	}
}

// fakePE builds a Windows PE header carrying machine.
func fakePE(machine uint16) []byte {
	head := make([]byte, 0x48)
	copy(head, "MZ")
	binary.LittleEndian.PutUint32(head[0x3c:], 0x40)
	copy(head[0x40:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(head[0x44:], machine)
	return head
}

// fakeLibrary returns a header that identifies both the format and the
// architecture of a platform's framework library, so the packer can verify a
// runtime belongs to the platform it is packed for.
func fakeLibrary(target platform.Target) []byte {
	switch target.OS {
	case platform.Windows:
		return fakePE(peMachine(target.Arch))
	case platform.Linux:
		return fakeELF(binary.LittleEndian, elfMachine(target.Arch))
	case platform.MacOS:
		return fakeMachO(machoCPU(target.Arch))
	default:
		return []byte("no header")
	}
}

func peMachine(arch platform.Arch) uint16 {
	if arch == platform.AArch64 {
		return 0xaa64
	}
	return 0x8664
}

func elfMachine(arch platform.Arch) uint16 {
	if arch == platform.AArch64 {
		return 183
	}
	return 62
}

func machoCPU(arch platform.Arch) uint32 {
	if arch == platform.AArch64 {
		return 0x0100000c
	}
	return 0x01000007
}

// fakeELF builds an ELF header carrying machine.
func fakeELF(order binary.ByteOrder, machine uint16) []byte {
	head := make([]byte, 20)
	copy(head, "\x7fELF")
	head[4] = 2 // 64-bit
	head[5] = 1 // little endian
	if order == binary.BigEndian {
		head[5] = 2
	}
	order.PutUint16(head[18:], machine)
	return head
}

// fakeMachO builds a thin Mach-O header carrying cpu.
func fakeMachO(cpu uint32) []byte {
	head := make([]byte, 32)
	binary.LittleEndian.PutUint32(head, 0xfeedfacf)
	binary.LittleEndian.PutUint32(head[4:], cpu)
	return head
}

// fakeFatMachO builds a universal binary header listing cpus.
func fakeFatMachO(cpus ...uint32) []byte {
	head := make([]byte, 8+len(cpus)*20)
	binary.BigEndian.PutUint32(head, 0xcafebabe)
	binary.BigEndian.PutUint32(head[4:], uint32(len(cpus)))
	for i, cpu := range cpus {
		binary.BigEndian.PutUint32(head[8+i*20:], cpu)
	}
	return head
}
