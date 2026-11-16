package vm

import (
	"errors"
	"slices"
	"testing"
)

// No test may sync the real /boot of the machine it runs on. A test that
// cares about the sync replaces this with countBootSyncs.
func init() {
	syncBootDir = func() error { return nil }
}

// countBootSyncs counts the syncs of /boot, and reports fail from each. Every
// command the test's run records must come before the sync, which ran is for.
func countBootSyncs(t *testing.T, fail error, ran *[]string, all int) *int {
	t.Helper()
	count := 0
	old := syncBootDir
	syncBootDir = func() error {
		if ran != nil && len(*ran) != all {
			t.Errorf("the sync ran after %v, want it after all %d commands", *ran, all)
		}
		count++
		return fail
	}
	t.Cleanup(func() { syncBootDir = old })
	return &count
}

// Every list that switches a function changes a marker, and each must end
// with a sync of /boot.
func TestEveryDeviceSwitchSyncsBoot(t *testing.T) {
	lists := map[string][]string{
		"network ncm": usbNetworkCommands(usbNetworkNCM),
		"network ecm": usbNetworkCommands(usbNetworkECM),
		"network off": usbNetworkCommands(usbNetworkOff),
	}
	for _, device := range []string{"console", "network", "disk", "audio"} {
		_, mount, unmount, ok := commandsFor(device)
		if !ok {
			t.Fatalf("no commands for %s", device)
		}
		lists[device+" on"] = mount
		lists[device+" off"] = unmount
	}

	for name, commands := range lists {
		var ran []string
		count := countBootSyncs(t, nil, &ran, len(commands))
		err := runMarkerCommands(commands, func(command string) error {
			ran = append(ran, command)
			return nil
		})
		if err != nil {
			t.Errorf("%s: %s", name, err)
		}
		if !slices.Equal(ran, commands) {
			t.Errorf("%s: ran %v, want %v", name, ran, commands)
		}
		if *count != 1 {
			t.Errorf("%s: /boot was synced %d times, want once", name, *count)
		}
	}
}

// The commands before a failed one are not undone, so the marker they changed
// is synced, and the failure is still the answer.
func TestAFailedSwitchStillSyncsBoot(t *testing.T) {
	failure := errors.New("exit status 1")
	var ran []string
	count := countBootSyncs(t, nil, &ran, 2)

	err := runMarkerCommands(mountDiskCommands, func(command string) error {
		ran = append(ran, command)
		if command == usbGadgetStop {
			return failure
		}
		return nil
	})
	if !errors.Is(err, failure) {
		t.Fatalf("got %v, want the command's failure", err)
	}
	if !slices.Equal(ran, []string{"touch /boot/usb.disk0", usbGadgetStop}) {
		t.Fatalf("ran %v", ran)
	}
	if *count != 1 {
		t.Fatalf("/boot was synced %d times, want once", *count)
	}
}

// The gadget carries the change already, so a sync that fails is not a failed
// switch.
func TestASwitchSucceedsWhenTheSyncFails(t *testing.T) {
	count := countBootSyncs(t, errors.New("input/output error"), nil, 0)

	if err := runMarkerCommands(unmountDiskCommands, func(string) error { return nil }); err != nil {
		t.Fatalf("switch: %s", err)
	}
	if *count != 1 {
		t.Fatalf("/boot was synced %d times, want once", *count)
	}
}

// A list with no marker in it, such as the rebuild a new subnet asks for,
// costs no flush of the card.
func TestCommandsWithoutAMarkerSyncNothing(t *testing.T) {
	count := countBootSyncs(t, nil, nil, 0)
	commands := []string{usbGadgetStop, "rm -rf /sys/kernel/config/usb_gadget/g0/configs/c.1/acm.GS0", usbGadgetStart}
	run := func(string) error { return nil }

	if err := runMarkerCommands(commands, run); err != nil {
		t.Fatal(err)
	}
	if err := runRebuild(commands, run); err != nil {
		t.Fatal(err)
	}
	if *count != 0 {
		t.Fatalf("/boot was synced %d times, want none", *count)
	}
}

func TestRunRebuildSyncsBootAfterTheStart(t *testing.T) {
	commands := []string{usbGadgetStop, "rm -f /boot/usb.acm", "touch /boot/usb.disk0", usbGadgetStart}
	var ran []string
	count := countBootSyncs(t, nil, &ran, len(commands))

	err := runRebuild(commands, func(command string) error {
		ran = append(ran, command)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if *count != 1 {
		t.Fatalf("/boot was synced %d times, want once", *count)
	}
}

// A step that fails leaves the markers before it changed, and start still
// runs: the sync comes after both.
func TestAFailedRebuildStillSyncsBoot(t *testing.T) {
	commands := []string{usbGadgetStop, "rm -f /boot/usb.acm", "touch /boot/usb.disk0", "touch /boot/usb.uac", usbGadgetStart}
	var ran []string
	count := countBootSyncs(t, nil, &ran, 4)

	err := runRebuild(commands, func(command string) error {
		ran = append(ran, command)
		if command == "touch /boot/usb.disk0" {
			return errors.New("read-only file system")
		}
		return nil
	})
	if err == nil {
		t.Fatal("a failed step was not reported")
	}
	if *count != 1 {
		t.Fatalf("/boot was synced %d times, want once", *count)
	}
}
