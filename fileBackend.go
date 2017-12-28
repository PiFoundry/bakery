package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
)

type fileBackend interface {
	GetNfsRoot() string
	GetNfsAddress() string
	GetBootRoot() string
	PutFileInNfsFolder(filePath string, content []byte) error
	MoveFilesToFolder(glob, dest string) error
	CreateNfsFolder(string) (string, error)
	DeleteNfsFolder(string) error
	GetNfsFolders(string) []string
	CopyNfsFolder(string, string) (string, error)
	CopyBootFolder(string, string) (string, error)
}

type FileBackend struct {
	nfsRoot        string
	nfsAddress     string
	bootRoot       string
	nfsExportMutex *sync.Mutex
}

func newFileBackend(nfsAddress, nfsRoot, bootRoot string) (fileBackend, error) {
	if nfsAddress == "" || nfsRoot == "" {
		return &FileBackend{}, fmt.Errorf("fileBackend: nfsAddress or nfsRoot not configured")
	}

	return &FileBackend{
		nfsRoot:        nfsRoot,
		nfsAddress:     nfsAddress,
		bootRoot:       bootRoot,
		nfsExportMutex: &sync.Mutex{},
	}, nil
}

func (nfs *FileBackend) GetNfsRoot() string {
	return nfs.nfsRoot
}

func (nfs *FileBackend) GetNfsAddress() string {
	return nfs.nfsAddress
}

func (nfs *FileBackend) GetBootRoot() string {
	return nfs.bootRoot
}

func (f *FileBackend) copyFolder(s, d string) error {
	_, err := exec.Command("rsync", "-xa", s, d).CombinedOutput()
	if err != nil {
		return err
	}

	return nil
}

func (f *FileBackend) PutFileInNfsFolder(filePath string, content []byte) error {
	fullFilePath := path.Join(f.nfsRoot, filePath)
	dir, _ := path.Split(fullFilePath)
	err := os.MkdirAll(dir, 0666)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fullFilePath, content, 0666)
}

func (f *FileBackend) CopyBootFolder(s, dest string) (string, error) {
	d := path.Join(f.bootRoot, dest)
	//d = strings.Replace(d, "//", "/", -1)
	return d, f.copyFolder(s, d)
}

func (f *FileBackend) CopyNfsFolder(s, dest string) (string, error) {
	d := path.Join(f.nfsRoot, dest)
	//d = strings.Replace(d, "//", "/", -1)
	err := f.copyFolder(s, d)
	if err != nil {
		return "", err
	}

	return d, f.regenNfsExports()
}

func (f *FileBackend) CreateNfsFolder(d string) (string, error) {
	location := path.Join(f.nfsRoot, d)
	err := os.Mkdir(location, 0644)
	if err != nil {
		return "", err
	}
	err = f.regenNfsExports()
	return location, err
}

func (f *FileBackend) DeleteNfsFolder(d string) error {
	location := path.Join(f.nfsRoot, d)
	err := os.RemoveAll(location)
	if err != nil {
		return err
	}
	return f.regenNfsExports()
}

func (f *FileBackend) MoveFilesToFolder(glob, dest string) error {
	files, _ := filepath.Glob(glob)
	for _, file := range files {
		err := os.Rename(file, dest+"/")
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *FileBackend) GetNfsFolders(pattern string) []string {
	files, _ := filepath.Glob(path.Join(f.nfsRoot, pattern))
	return files
}

func (f *FileBackend) regenNfsExports() error {
	f.nfsExportMutex.Lock()
	defer f.nfsExportMutex.Unlock()

	folderList := f.GetNfsFolders("*")

	exportsContent := ""
	for _, folder := range folderList {
		exportsContent = exportsContent + fmt.Sprintf("%v *(rw,sync,no_subtree_check,no_root_squash)\n", folder)
	}

	fmt.Println("Generated new exports file:\n" + exportsContent)
	err := ioutil.WriteFile("/etc/exports", []byte(exportsContent), 0644)
	if err != nil {
		return err
	}

	//	_, err = exec.Command("exportfs", "-a").CombinedOutput()
	return err
}
