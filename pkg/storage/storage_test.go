package storage

import (
	"os"
	"testing"

	"plant-shutter-pi/pkg/storage/model"
)

func TestStorage_ProjectLifecycle(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatalf("New storage: %v", err)
	}

	cam := model.CameraSettings{}

	// create project
	p, err := s.NewProject("p1", "desc", 1000, cam)
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	if _, err := os.Stat(p.GetRootPath()); err != nil {
		t.Fatalf("project dir not created: %v", err)
	}

	// duplicate should fail
	if _, err := s.NewProject("p1", "desc2", 1000, cam); err == nil {
		t.Fatalf("expected duplicate project error")
	}

	// get project
	got, err := s.GetProject("p1")
	if err != nil || got == nil {
		t.Fatalf("GetProject want non-nil, got (%v,%v)", got, err)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt should be set")
	}

	// update project (CreatedAt should persist)
	oldCreated := got.CreatedAt
	got.Info = "updated"
	if err := s.UpdateProject(got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	got2, err := s.GetProject("p1")
	if err != nil || got2 == nil {
		t.Fatalf("GetProject after update: (%v,%v)", got2, err)
	}
	if got2.Info != "updated" || !got2.CreatedAt.Equal(oldCreated) {
		t.Fatalf("UpdateProject did not persist fields correctly")
	}

	// last running project
	if err := s.SetLastRunningProject("p1"); err != nil {
		t.Fatalf("SetLastRunningProject: %v", err)
	}
	last, err := s.GetLastRunningProject()
	if err != nil || last == nil || last.Name != "p1" {
		t.Fatalf("GetLastRunningProject: (%v,%v)", last, err)
	}
	if err := s.ClearLastRunningProject(); err != nil {
		t.Fatalf("ClearLastRunningProject: %v", err)
	}
	none, err := s.GetLastRunningProject()
	if err != nil || none != nil {
		t.Fatalf("GetLastRunningProject after clear want nil, got (%v,%v)", none, err)
	}

	// Save an image to the project to verify FS removal
	if err := got2.SaveImage([]byte("img")); err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	if got2.ImageCount != 1 {
		t.Fatalf("ImageCount want 1, got %d", got2.ImageCount)
	}

	// delete project: removes from DB and filesystem
	if err := s.DeleteProject("p1"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	// directory should be gone
	if _, err := os.Stat(got2.GetRootPath()); !os.IsNotExist(err) {
		t.Fatalf("project dir still exists or unexpected err: %v", err)
	}
	// DB should not return the project anymore
	gone, err := s.GetProject("p1")
	if err != nil || gone != nil {
		t.Fatalf("GetProject after delete want nil, got (%v,%v)", gone, err)
	}
}
