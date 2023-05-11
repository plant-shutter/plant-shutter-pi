package storage

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/goccy/go-json"

	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/storage/project"
	"plant-shutter-pi/pkg/storage/util"
)

type Storage struct {
	rootDir string
}

func New(path string) (*Storage, error) {
	if path == "" {
		return nil, fmt.Errorf("rootDir can not be empty")
	}

	if err := util.MkdirAll(path); err != nil {
		return nil, err
	}

	s := &Storage{rootDir: path}
	if err := s.initInfoFile(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) Close() error {
	return nil
}

// ListProjects without bind
func (s *Storage) ListProjects() ([]*project.Project, error) {
	data, err := os.ReadFile(s.getProjectInfoPath())
	if err != nil {
		return nil, err
	}
	var list []*project.Project

	if err = json.Unmarshal(data, &list); err != nil {
		return nil, err
	}

	return list, nil
}

func (s *Storage) GetProject(name string) (*project.Project, error) {
	list, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.Name == name {
			s.bindProject(p)
			return p, nil
		}
	}

	return nil, nil
}

func (s *Storage) NewProject(name, info string, interval time.Duration) (*project.Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name can not be empty")
	}
	if interval < consts.MinInterval {
		return nil, fmt.Errorf("interval %s less than %s", interval, consts.MinInterval)
	}

	list, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.Name == name {
			return nil, fmt.Errorf("project name already exists")
		}
	}
	p, err := project.New(name, info, interval)
	if err != nil {
		return nil, err
	}
	s.bindProject(p)
	list = append(list, p)

	return p, s.dump(list)
}

func (s *Storage) UpdateProject(p *project.Project) error {
	if p == nil {
		return fmt.Errorf("project can not be nil")
	}
	list, err := s.ListProjects()
	if err != nil {
		return err
	}
	for i := 0; i < len(list); i++ {
		if list[i].Name == p.Name {
			list[i] = p
			return s.dump(list)
		}
	}

	return fmt.Errorf("project does not exist")
}

func (s *Storage) dump(list []*project.Project) error {
	f, err := os.Create(s.getProjectInfoPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(list)
}

func (s *Storage) getProjectInfoPath() string {
	return path.Join(s.rootDir, consts.DefaultInfoFile)
}

func (s *Storage) initInfoFile() error {
	_, err := os.Stat(s.getProjectInfoPath())
	if os.IsNotExist(err) {
		return s.dump(make([]*project.Project, 0))
	}

	return err
}

func (s *Storage) bindProject(p *project.Project) {
	p.SetRootDir(s.rootDir)
}
