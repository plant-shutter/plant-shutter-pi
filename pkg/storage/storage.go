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

type LastInfo struct {
	LastRunning *project.Project
}

func New(path string) (*Storage, error) {
	if path == "" {
		return nil, fmt.Errorf("rootDir can not be empty")
	}

	if err := util.MkdirAll(path); err != nil {
		return nil, err
	}

	s := &Storage{rootDir: path}
	if err := s.initDependFile(); err != nil {
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
			p.SetRootDir(s.rootDir)
			return p, nil
		}
	}

	return nil, nil
}

func (s *Storage) NewProject(name, info string, interval time.Duration) (*project.Project, error) {
	list, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.Name == name {
			return nil, fmt.Errorf("project name already exists")
		}
	}
	p, err := project.New(name, info, interval, s.rootDir)
	if err != nil {
		return nil, err
	}
	list = append(list, p)
	p.SetRootDir(s.rootDir)

	return p, s.dumpList(list)
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
			p.CreatedAt = list[i].CreatedAt
			list[i] = p
			return s.dumpList(list)
		}
	}

	return fmt.Errorf("project does not exist")
}

func (s *Storage) DeleteProject(name string) error {
	list, err := s.ListProjects()
	if err != nil {
		return err
	}
	for i := 0; i < len(list); i++ {
		if list[i].Name == name {
			p := list[i]
			p.SetRootDir(s.rootDir)
			list = append(list[:i], list[i+1:]...)
			if err = s.dumpList(list); err != nil {
				return err
			}
			return p.Clear()
		}
	}

	return nil
}

func (s *Storage) GetLastRunningProject() (*project.Project, error) {
	data, err := os.ReadFile(s.getProjectLastRunningPath())
	if err != nil {
		return nil, err
	}
	p := &LastInfo{}
	err = json.Unmarshal(data, p)
	if err != nil {
		return nil, err
	}
	p.LastRunning.SetRootDir(s.rootDir)

	return p.LastRunning, nil
}

func (s *Storage) SetLastRunningProject(name string) error {
	p, err := s.GetProject(name)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("project does not exist")
	}

	return s.dumpLastRunning(LastInfo{LastRunning: p})
}

func (s *Storage) dumpList(list []*project.Project) error {
	f, err := os.Create(s.getProjectInfoPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(list)
}

func (s *Storage) dumpLastRunning(p LastInfo) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	return os.WriteFile(s.getProjectLastRunningPath(), data, 0660)
}

func (s *Storage) getProjectInfoPath() string {
	return path.Join(s.rootDir, consts.DefaultInfoFile)
}

func (s *Storage) getProjectLastRunningPath() string {
	return path.Join(s.rootDir, consts.DefaultLastRunningFile)
}

func (s *Storage) initDependFile() error {
	_, err := os.Stat(s.getProjectInfoPath())
	if os.IsNotExist(err) {
		if err = s.dumpList(make([]*project.Project, 0)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = os.Stat(s.getProjectLastRunningPath())
	if os.IsNotExist(err) {
		return s.dumpLastRunning(LastInfo{})
	}

	return err
}
