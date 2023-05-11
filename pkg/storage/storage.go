package storage

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/goccy/go-json"
)

var (
	storagePath string
)

func Init(path string) error {
	if path == "" {
		return fmt.Errorf("storagePath can not be empty")
	}
	storagePath = path

	if err := mkdirAll(storagePath); err != nil {
		return err
	}

	if err := checkInitInfo(); err != nil {
		return err
	}

	return nil
}

func Close() error {
	return nil
}

func ListProjects() ([]*Project, error) {
	data, err := os.ReadFile(getProjectInfoPath())
	if err != nil {
		return nil, err
	}
	var list []*Project

	if err = json.Unmarshal(data, &list); err != nil {
		return nil, err
	}

	return list, nil
}

func GetProject(name string) (*Project, error) {
	list, err := ListProjects()
	if err != nil {
		return nil, err
	}
	for _, project := range list {
		if project.Name == name {
			return project, nil
		}
	}

	return nil, nil
}

func NewProject(name, info string) (*Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name can not be empty")
	}

	list, err := ListProjects()
	if err != nil {
		return nil, err
	}
	for _, project := range list {
		if project.Name == name {
			return nil, fmt.Errorf("project name already exists")
		}
	}
	p := &Project{
		Name:      name,
		Info:      info,
		CreatedAt: time.Now(),
	}
	if err := p.init(); err != nil {
		return nil, err
	}
	list = append(list, p)

	return p, dumpProjectInfo(list)
}

func dumpProjectInfo(list []*Project) error {
	f, err := os.Create(getProjectInfoPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(list)
}

func getProjectInfoPath() string {
	return path.Join(storagePath, DefaultInfoFile)
}

func checkInitInfo() error {
	_, err := os.Stat(getProjectInfoPath())
	if os.IsNotExist(err) {
		return dumpProjectInfo(make([]*Project, 0))
	}

	return err
}

//func getMD5(data []byte) string {
//	return fmt.Sprintf("%x", md5.Sum(data))
//}
