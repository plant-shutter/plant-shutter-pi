package storage

import (
	"fmt"
	"path"

	"github.com/objectbox/objectbox-go/objectbox"

	"plant-shutter-pi/pkg/storage/model"
	"plant-shutter-pi/pkg/utils"
)

type Storage struct {
	rootDir    string
	obx        *objectbox.ObjectBox
	projectBox *model.ProjectEntityBox
	lastBox    *model.LastRunningEntityBox
}

func New(root string) (*Storage, error) {
	if root == "" {
		return nil, fmt.Errorf("rootDir can not be empty")
	}

	if err := utils.MkdirAll(root); err != nil {
		return nil, err
	}

	// Initialize ObjectBox store under the storage root directory
	ob, err := objectbox.NewBuilder().
		Model(model.ObjectBoxModel()).
		Directory(path.Join(root, "objectbox")).
		Build()
	if err != nil {
		return nil, err
	}

	s := &Storage{
		rootDir:    root,
		obx:        ob,
		projectBox: model.BoxForProjectEntity(ob),
		lastBox:    model.BoxForLastRunningEntity(ob),
	}

	return s, nil
}

// ListProjects returns all projects from ObjectBox.
func (s *Storage) ListProjects() ([]*model.Project, error) {
	entities, err := s.projectBox.GetAll()
	if err != nil {
		return nil, err
	}
	res := make([]*model.Project, 0, len(entities))
	for _, e := range entities {
		p := &model.Project{
			ProjectEntity: *e,
		}
		p.SetRootDir(s.rootDir)
		res = append(res, p)
	}
	return res, nil
}

func (s *Storage) GetProject(name string) (*model.Project, error) {
	q := s.projectBox.Query(model.ProjectEntity_.Name.Equals(name, true))
	defer q.Close()
	list, err := q.Find()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	p := &model.Project{
		ProjectEntity: *list[0],
	}
	p.SetRootDir(s.rootDir)

	return p, nil
}

func (s *Storage) NewProject(name, info string, interval int32, camera model.CameraSettings, video model.VideoSetting) (*model.Project, error) {
	existing, err := s.GetProject(name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("project name already exists")
	}

	p, err := model.New(name, info, interval, s.rootDir, camera, video)
	if err != nil {
		return nil, err
	}
	if err = s.putProject(p); err != nil {
		return nil, err
	}
	p.SetRootDir(s.rootDir)
	return p, nil
}

func (s *Storage) UpdateProject(p *model.Project) error {
	if p == nil {
		return fmt.Errorf("project can not be nil")
	}
	// Ensure it exists to preserve CreatedAt
	old, err := s.GetProject(p.Name)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("project does not exist")
	}
	p.CreatedAt = old.CreatedAt
	return s.putProject(p)
}

func (s *Storage) DeleteProject(name string) error {
	q := s.projectBox.Query(model.ProjectEntity_.Name.Equals(name, true))
	defer q.Close()
	list, err := q.Find()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil
	}
	// remove from DB
	if _, err = s.projectBox.RemoveMany(list...); err != nil {
		return err
	}
	// remove from filesystem
	p := &model.Project{
		ProjectEntity: *list[0],
	}
	p.SetRootDir(s.rootDir)
	return p.Clear()
}

func (s *Storage) GetLastRunningProject() (*model.Project, error) {
	list, err := s.lastBox.GetAll()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 || list[0].ProjectName == "" {
		return nil, nil
	}
	return s.GetProject(list[0].ProjectName)
}

func (s *Storage) ClearLastRunningProject() error {
	// Remove all rows
	all, err := s.lastBox.GetAll()
	if err != nil {
		return err
	}
	if len(all) > 0 {
		_, err = s.lastBox.RemoveMany(all...)
	}
	return err
}

func (s *Storage) SetLastRunningProject(name string) error {
	p, err := s.GetProject(name)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("project does not exist")
	}
	// only keep a single row
	if err = s.ClearLastRunningProject(); err != nil {
		return err
	}
	_, err = s.lastBox.Put(&model.LastRunningEntity{ProjectName: name})
	return err
}

// putProject stores/updates the project configuration entity
func (s *Storage) putProject(p *model.Project) error {
	if p == nil {
		return fmt.Errorf("project can not be nil")
	}
	_, err := s.projectBox.Put(&p.ProjectEntity)
	return err
}
