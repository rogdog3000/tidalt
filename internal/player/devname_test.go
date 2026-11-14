package player

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func fakeAsound(t *testing.T, cards map[int]string) {
	t.Helper()
	root := t.TempDir()
	for n, id := range cards {
		dir := filepath.Join(root, "card"+strconv.Itoa(n))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "id"), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := asoundRoot
	asoundRoot = root
	t.Cleanup(func() { asoundRoot = old })
}

// TestDeviceNameSurvivesRenumbering: a DAC saved while it was card 5 must
// resolve to its new number after a reboot, not to whatever is now card 5.
func TestDeviceNameSurvivesRenumbering(t *testing.T) {
	fakeAsound(t, map[int]string{0: "Display", 5: "Amplifier"})
	stable := StableDeviceName("hw:5,0")
	if stable != "hw:CARD=Amplifier,DEV=0" {
		t.Fatalf("stable name = %q", stable)
	}

	fakeAsound(t, map[int]string{0: "Amplifier", 5: "Display"})
	if got := ResolveDevice(stable); got != "hw:0,0" {
		t.Errorf("resolved = %q, want hw:0,0", got)
	}
}

func TestDeviceNamePassthrough(t *testing.T) {
	fakeAsound(t, map[int]string{1: "NVidia"})
	for _, in := range []string{"", "default", "hw:1,0", "hw:CARD=Gone,DEV=0"} {
		if got := ResolveDevice(in); got != in {
			t.Errorf("ResolveDevice(%q) = %q", in, got)
		}
	}
	if got := StableDeviceName("hw:9,0"); got != "hw:9,0" {
		t.Errorf("unknown card should pass through, got %q", got)
	}
}
