package storage

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// recordSyncs replaces syncDir with one that records each directory, and
// fails the test when driveMu is held during the call: a sync of exFAT flushes
// the card, and every drive request would wait behind it. It also checks that
// the entry change is already made, with seen.
func recordSyncs(t *testing.T, seen func(), fail error) *[]string {
	t.Helper()
	var dirs []string
	old := syncDir
	syncDir = func(dir string) error {
		if !driveMu.TryLock() {
			t.Errorf("sync of %s runs under driveMu", dir)
		} else {
			driveMu.Unlock()
		}
		if seen != nil {
			seen()
		}
		dirs = append(dirs, dir)
		return fail
	}
	t.Cleanup(func() { syncDir = old })
	return &dirs
}

func TestRemoveImageSyncsTheDirectoryOutsideTheLock(t *testing.T) {
	fakeGadget(t, "lun.0", "lun.1")
	dir := t.TempDir()
	image := filepath.Join(dir, "a.iso")
	if err := os.WriteFile(image, []byte("iso"), 0o666); err != nil {
		t.Fatalf("setup: %s", err)
	}
	dirs := recordSyncs(t, func() {
		if _, err := os.Stat(image); !os.IsNotExist(err) {
			t.Errorf("the sync ran before the remove: %v", err)
		}
	}, nil)

	if err := removeImage(image); err != nil {
		t.Fatalf("remove: %s", err)
	}
	if !reflect.DeepEqual(*dirs, []string{dir}) {
		t.Fatalf("synced %v, want %v", *dirs, []string{dir})
	}
}

// The image is gone from the running system either way, so a failed sync is
// not a failed delete.
func TestRemoveImageSucceedsWhenTheSyncFails(t *testing.T) {
	fakeGadget(t, "lun.0", "lun.1")
	image := filepath.Join(t.TempDir(), "a.iso")
	if err := os.WriteFile(image, []byte("iso"), 0o666); err != nil {
		t.Fatalf("setup: %s", err)
	}
	dirs := recordSyncs(t, nil, errors.New("input/output error"))

	if err := removeImage(image); err != nil {
		t.Fatalf("remove: %s", err)
	}
	if len(*dirs) != 1 {
		t.Fatalf("synced %v", *dirs)
	}
}

func TestARefusedRemoveSyncsNothing(t *testing.T) {
	fakeGadget(t, "lun.0", "lun.1")
	image := filepath.Join(t.TempDir(), "a.iso")
	if err := os.WriteFile(image, []byte("iso"), 0o666); err != nil {
		t.Fatalf("setup: %s", err)
	}
	if err := os.WriteFile(filepath.Join(massStorageDir, "lun.1", "file"), []byte(image+"\n"), 0o666); err != nil {
		t.Fatalf("setup: %s", err)
	}
	dirs := recordSyncs(t, nil, nil)

	if err := removeImage(image); !errors.Is(err, errImageLoaded) {
		t.Fatalf("got %v, want errImageLoaded", err)
	}
	if len(*dirs) != 0 {
		t.Fatalf("synced %v", *dirs)
	}
}

func TestReplaceImageSyncsTheDirectoryOutsideTheLock(t *testing.T) {
	for _, hid := range []bool{false, true} {
		fakeGadget(t, "lun.0", "lun.1")
		useHidOnly(t, hid)
		dir := t.TempDir()
		src := filepath.Join(dir, ".download")
		image := filepath.Join(dir, "a.iso")
		if err := os.WriteFile(src, []byte("new"), 0o666); err != nil {
			t.Fatalf("setup: %s", err)
		}
		dirs := recordSyncs(t, func() {
			if data, err := os.ReadFile(image); err != nil || string(data) != "new" {
				t.Errorf("the sync ran before the rename: %q, %v", data, err)
			}
		}, nil)

		if err := ReplaceImage(src, image); err != nil {
			t.Fatalf("replace, HID only %v: %s", hid, err)
		}
		if !reflect.DeepEqual(*dirs, []string{dir}) {
			t.Fatalf("HID only %v: synced %v, want %v", hid, *dirs, []string{dir})
		}
	}
}

func TestInstallImageSyncsTheDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, ".upload")
	image := filepath.Join(dir, "a.iso")
	if err := os.WriteFile(src, []byte("new"), 0o666); err != nil {
		t.Fatalf("setup: %s", err)
	}
	dirs := recordSyncs(t, nil, nil)

	if err := InstallImage(src, image); err != nil {
		t.Fatalf("install: %s", err)
	}
	if !reflect.DeepEqual(*dirs, []string{dir}) {
		t.Fatalf("synced %v, want %v", *dirs, []string{dir})
	}

	if err := InstallImage(src, image); err == nil {
		t.Fatal("a rename of a missing file succeeded")
	}
	if len(*dirs) != 1 {
		t.Fatalf("a failed rename synced: %v", *dirs)
	}
}

func TestCreateImageSyncsTheDirectory(t *testing.T) {
	root := createFixture(t)
	dirs := recordSyncs(t, nil, nil)

	if rsp := createCall(t, "exchange", 64<<20, ""); rsp.Code != 0 {
		t.Fatalf("create: %d %s", rsp.Code, rsp.Msg)
	}
	if !reflect.DeepEqual(*dirs, []string{root}) {
		t.Fatalf("synced %v, want %v", *dirs, []string{root})
	}

	if rsp := createCall(t, "exchange", 64<<20, ""); rsp.Code == 0 {
		t.Fatal("an existing name was created again")
	}
	if len(*dirs) != 1 {
		t.Fatalf("a refused create synced: %v", *dirs)
	}
}
