//go:build linux

package diskfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenBackingRefusesASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.img")
	if err := os.Symlink(image, link); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []int{unix.O_RDONLY, unix.O_RDWR} {
		fd, err := openBacking(image, mode)
		if err != nil {
			t.Fatalf("a regular image was refused: %s", err)
		}
		_ = unix.Close(fd)

		fd, err = openBacking(link, mode)
		if err == nil {
			_ = unix.Close(fd)
			t.Fatal("a symbolic link was opened as an image")
		}
		if !errors.Is(err, unix.ELOOP) {
			t.Fatalf("got %v, want ELOOP", err)
		}
	}
}

// Mount tracks an image by its resolved path and attaches that, so a link the
// user keeps under /data still opens: only its target reaches openBacking.
func TestAResolvedLinkStillOpens(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.img")
	if err := os.Symlink(image, link); err != nil {
		t.Fatal(err)
	}

	fd, err := openBacking(keyOf(link), unix.O_RDONLY)
	if err != nil {
		t.Fatalf("the resolved link was refused: %s", err)
	}
	_ = unix.Close(fd)
}

// attachLoop must fail on the link before it takes a loop device.
func TestAttachLoopRefusesASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(image, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.img")
	if err := os.Symlink(image, link); err != nil {
		t.Fatal(err)
	}

	loop, err := attachLoop(link, 0, 0, false)
	if err == nil {
		loop.Detach()
		t.Fatal("a symbolic link was attached to a loop device")
	}
	if !errors.Is(err, unix.ELOOP) {
		t.Fatalf("got %v, want ELOOP", err)
	}
}
