package vm

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"NanoKVM-Server/config"
	"NanoKVM-Server/utils/gpiocdev"
)

func useNoGpioChips(t *testing.T) {
	t.Helper()
	old := gpiocdev.ChipGlob
	gpiocdev.ChipGlob = filepath.Join(t.TempDir(), "gpiochip*")
	t.Cleanup(func() { gpiocdev.ChipGlob = old })
}

// On the vendor kernel the device tree names no lines. A sysfs path that is
// missing there must fail as it always has, with the file's own error, and not
// with something the character-device fallback made up.
func TestMissingSysfsLineWithoutANamedLineFailsAsBefore(t *testing.T) {
	useNoGpioChips(t)
	missing := filepath.Join(t.TempDir(), "gpio503", "value")
	conf := &config.GetInstance().Hardware
	old := *conf
	t.Cleanup(func() { *conf = old })
	conf.GPIOPower = missing
	conf.GPIOPowerLED = missing + ".led"

	if err := writeGpio(missing, time.Millisecond); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("writeGpio on a missing path = %v, want the not-exist error", err)
	}
	if _, err := readGpio(conf.GPIOPowerLED); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("readGpio on a missing path = %v, want the not-exist error", err)
	}
}

// On the vendor kernel the startup claim leaves the sysfs lines to S15kvmhwd:
// it neither writes them nor caches a character-device line for them.
func TestHoldButtonsReleasedLeavesSysfsLinesAlone(t *testing.T) {
	power, reset, _ := useButtons(t, "0\n")
	for _, path := range []string{power, reset} {
		if err := os.WriteFile(path, []byte("1"), 0o600); err != nil {
			t.Fatalf("failed to seed %s: %s", path, err)
		}
	}

	HoldButtonsReleased()

	for _, path := range []string{power, reset} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "1" {
			t.Fatalf("%s = %q, %v after HoldButtonsReleased, want it untouched", path, got, err)
		}
	}
	assertNoCdevLines(t)
}

// With no sysfs path and no chip that names the lines, the startup claim is a
// no-op: the first press still fails the way it did before.
func TestHoldButtonsReleasedWithoutNamedLinesDoesNothing(t *testing.T) {
	useNoGpioChips(t)
	dir := t.TempDir()
	conf := &config.GetInstance().Hardware
	old := *conf
	t.Cleanup(func() { *conf = old })
	conf.GPIOPower = filepath.Join(dir, "gpio503", "value")
	conf.GPIOReset = filepath.Join(dir, "gpio505", "value")

	HoldButtonsReleased()

	assertNoCdevLines(t)
	if err := writeGpio(conf.GPIOPower, time.Millisecond); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("writeGpio after HoldButtonsReleased = %v, want the not-exist error", err)
	}
}

func assertNoCdevLines(t *testing.T) {
	t.Helper()
	cdevLinesMu.Lock()
	defer cdevLinesMu.Unlock()
	if len(cdevLines) != 0 {
		t.Fatalf("cdevLines = %v, want none", cdevLines)
	}
}

// An existing sysfs path never consults the character device.
func TestExistingSysfsPathIsNotReplaced(t *testing.T) {
	_, _, led := useButtons(t, "0\n")
	if line := cdevLine(led, gpiocdev.Input); line != nil {
		t.Fatalf("cdevLine(%q) = %v, want the sysfs path", led, line.Name())
	}
}
