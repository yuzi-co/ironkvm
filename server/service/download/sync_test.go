package download

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"NanoKVM-Server/service/netboot"
)

// recordTransferSteps replaces the two steps that make a transfer survive a
// power cut with ones that record their order. The sync must see the whole
// image in the temporary file, and the install must come after it.
func recordTransferSteps(t *testing.T, want []byte, syncErr error) *[]string {
	t.Helper()
	var steps []string
	oldSync, oldInstall := syncFile, installImage
	t.Cleanup(func() { syncFile, installImage = oldSync, oldInstall })

	syncFile = func(file *os.File) error {
		name := filepath.Base(file.Name())
		if !strings.HasPrefix(name, ".nanokvm-") {
			t.Errorf("synced %s, want the temporary file", name)
		}
		if data, err := os.ReadFile(file.Name()); err != nil || !bytes.Equal(data, want) {
			t.Errorf("the sync saw %d bytes (%v), want %d", len(data), err, len(want))
		}
		steps = append(steps, "sync")
		return syncErr
	}
	installImage = func(tmp, dest string) error {
		steps = append(steps, "install "+filepath.Base(dest))
		return oldInstall(tmp, dest)
	}
	return &steps
}

func TestAnUploadIsSyncedBeforeItGetsItsName(t *testing.T) {
	body := headWithDescriptor("CD001")
	body = append(body, bytes.Repeat([]byte{0x5A}, 100_000)...)
	steps := recordTransferSteps(t, body, nil)

	if code := upload(t, "a.iso", body); code != 0 {
		t.Fatalf("a valid image was refused, code %d", code)
	}
	if want := []string{"sync", "install a.iso"}; !reflect.DeepEqual(*steps, want) {
		t.Fatalf("steps %v, want %v", *steps, want)
	}
}

// An image whose bytes did not reach the card must not get a name.
func TestAnUploadThatCannotBeSyncedLeavesNothing(t *testing.T) {
	body := headWithDescriptor("CD001")
	steps := recordTransferSteps(t, body, errors.New("input/output error"))

	if code := upload(t, "a.iso", body); code == 0 {
		t.Fatal("an upload that was not synced was accepted")
	}
	if want := []string{"sync"}; !reflect.DeepEqual(*steps, want) {
		t.Fatalf("steps %v, want %v", *steps, want)
	}
	if names := imageFiles(t); len(names) != 0 {
		t.Fatalf("a failed upload left %v", names)
	}
}

// A refused upload has nothing worth a flush of the card.
func TestARefusedUploadIsNotSynced(t *testing.T) {
	body := bytes.Repeat([]byte{0xA5}, imageHeaderSize+1000)
	steps := recordTransferSteps(t, body, nil)

	if code := upload(t, "c.iso", body); code == 0 {
		t.Fatal("a file that is no image was accepted")
	}
	if len(*steps) != 0 {
		t.Fatalf("steps %v, want none", *steps)
	}
}

func TestADownloadIsSyncedBeforeItIsInstalled(t *testing.T) {
	scratchBootMenu(t)
	url := serveISO(t, "new iso")
	steps := recordTransferSteps(t, []byte("new iso"), nil)

	err := NewService().downloadRemoteImage(context.Background(), url, nil, netboot.BootMenuISOName, nil, func(tmp, dest string) error {
		*steps = append(*steps, "install "+filepath.Base(dest))
		return os.Rename(tmp, dest)
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"sync", "install " + netboot.BootMenuISOName}; !reflect.DeepEqual(*steps, want) {
		t.Fatalf("steps %v, want %v", *steps, want)
	}
}

func TestADownloadThatCannotBeSyncedIsNotInstalled(t *testing.T) {
	scratchBootMenu(t)
	url := serveISO(t, "new iso")
	recordTransferSteps(t, []byte("new iso"), errors.New("input/output error"))

	err := NewService().downloadRemoteImage(context.Background(), url, nil, netboot.BootMenuISOName, nil, func(tmp, dest string) error {
		t.Error("a download that was not synced was installed")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "sync") {
		t.Fatalf("download: %v", err)
	}
	if entries, _ := os.ReadDir(imageDir); len(entries) != 0 {
		t.Fatalf("a failed download left %d entries", len(entries))
	}
}

// The boot menu update installs through storage.ReplaceImage, which syncs the
// directory itself, so only the sync of the bytes shows here.
func TestTheBootMenuUpdateSyncsTheISO(t *testing.T) {
	scratchBootMenu(t)
	url := serveISO(t, "new iso")
	steps := recordTransferSteps(t, []byte("new iso"), nil)

	next := bootMenu{Version: "9.9.9", URL: url, SHA256: sumOf("new iso")}
	if err := NewService().updateBootMenu(context.Background(), next, func(int) {}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"sync"}; !reflect.DeepEqual(*steps, want) {
		t.Fatalf("steps %v, want %v", *steps, want)
	}
}

// A download with the wrong checksum is removed, so the card is not flushed
// for it.
func TestADownloadWithTheWrongChecksumIsNotSynced(t *testing.T) {
	scratchBootMenu(t)
	url := serveISO(t, "tampered iso")
	steps := recordTransferSteps(t, []byte("tampered iso"), nil)

	next := bootMenu{Version: "9.9.9", URL: url, SHA256: sumOf("new iso")}
	if err := NewService().updateBootMenu(context.Background(), next, func(int) {}); err == nil {
		t.Fatal("a tampered ISO was installed")
	}
	if len(*steps) != 0 {
		t.Fatalf("steps %v, want none", *steps)
	}
}
