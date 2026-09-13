package model

import (
	"io/fs"
	"os"
	"path"
	"testing"
)

func newTestProject(t *testing.T, name string) *Project {
	t.Helper()
	root := t.TempDir()
	cam := CameraSettings{}
	p, err := New(name, "info", 5000, root, cam)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return p
}

func TestNewProject_InitStorage(t *testing.T) {
	p := newTestProject(t, "projA")

	// root directory should be <root>/<name>
	if got, wantSuffix := p.GetRootPath(), path.Join(path.Dir(p.GetRootPath()), p.Name); got != wantSuffix {
		t.Fatalf("unexpected root path: got=%q", got)
	}

	// images directory should exist
	if fi, err := os.Stat(p.getImageDirPath()); err != nil || !fi.IsDir() {
		t.Fatalf("images dir not created: %v", err)
	}

	if p.ImageCount != 0 || p.LatestImageName != "" {
		t.Fatalf("unexpected initial state: count=%d latest=%q", p.ImageCount, p.LatestImageName)
	}
}

func TestSaveAndGetImage(t *testing.T) {
	p := newTestProject(t, "projB")

	img1 := []byte("image-one")
	if err := p.SaveImage(img1); err != nil {
		t.Fatalf("SaveImage(1) error = %v", err)
	}

	if p.ImageCount != 1 {
		t.Fatalf("ImageCount want 1, got %d", p.ImageCount)
	}

	// verify naming format
	wantName1 := "projB-0000000.jpg"
	if p.LatestImageName != wantName1 {
		t.Fatalf("LatestImageName want %q, got %q", wantName1, p.LatestImageName)
	}

	// GetLatestImageName
	if name, err := p.GetLatestImageName(); err != nil || name != wantName1 {
		t.Fatalf("GetLatestImageName = (%q,%v), want (%q,nil)", name, err, wantName1)
	}

	// GetLatestImage content
	if got, err := p.GetLatestImage(); err != nil || string(got) != string(img1) {
		t.Fatalf("GetLatestImage = (%q,%v), want (%q,nil)", string(got), err, string(img1))
	}

	// Save second image
	img2 := []byte("image-two")
	if err := p.SaveImage(img2); err != nil {
		t.Fatalf("SaveImage(2) error = %v", err)
	}

	wantName2 := "projB-0000001.jpg"
	if p.ImageCount != 2 || p.LatestImageName != wantName2 {
		t.Fatalf("after second save: count=%d latest=%q", p.ImageCount, p.LatestImageName)
	}

	// ListImages should iterate only .jpg files
	var seen []string
	err := p.ListImages(func(info fs.FileInfo) error {
		seen = append(seen, info.Name())
		return nil
	})
	if err != nil {
		t.Fatalf("ListImages error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("ListImages count want 2, got %d (%v)", len(seen), seen)
	}
}

func TestListFiles_FiltersAndIgnoresDirs(t *testing.T) {
	p := newTestProject(t, "projC")

	// create non-matching files and subdir
	if err := os.WriteFile(path.Join(p.getImageDirPath(), "note.txt"), []byte("x"), 0o666); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if err := os.MkdirAll(path.Join(p.getImageDirPath(), "sub"), 0o777); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path.Join(p.getImageDirPath(), "sub", "ignored.jpg"), []byte("y"), 0o666); err != nil {
		t.Fatalf("write nested jpg: %v", err)
	}

	// create matching images via API
	_ = p.SaveImage([]byte("a"))
	_ = p.SaveImage([]byte("b"))

	var count int
	if err := p.ListImages(func(info fs.FileInfo) error { count++; return nil }); err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if count != 2 {
		t.Fatalf("ListImages count want 2, got %d", count)
	}
}

func TestGetImage_NotFound(t *testing.T) {
	p := newTestProject(t, "projD")
	if _, err := p.GetImage("missing.jpg"); err == nil {
		t.Fatalf("GetImage expected error for missing file")
	}
}

func TestClearImages(t *testing.T) {
	p := newTestProject(t, "projE")
	if err := p.SaveImage([]byte("x")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := p.ClearImages(); err != nil {
		t.Fatalf("ClearImages: %v", err)
	}
	// images dir should exist and be empty
	entries, err := os.ReadDir(p.getImageDirPath())
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("images dir not empty after ClearImages: %d", len(entries))
	}

	// ImageCount is not reset by ClearImages; next save should increment number
	if err := p.SaveImage([]byte("y")); err != nil {
		t.Fatalf("save after clear: %v", err)
	}
	if p.ImageCount != 2 {
		t.Fatalf("ImageCount want 2 after re-save, got %d", p.ImageCount)
	}
}

func TestClear_RemovesRoot(t *testing.T) {
	p := newTestProject(t, "projF")
	if err := p.SaveImage([]byte("z")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := p.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(p.GetRootPath()); !os.IsNotExist(err) {
		t.Fatalf("project root still exists or unexpected err: %v", err)
	}
}
