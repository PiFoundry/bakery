package main

import (
	"fmt"
	"os/exec"
)

type fileBackend interface {
	GetNfsRoot() string
	GetNfsAddress() string
	GetBootRoot() string
	CopyFolder(s, d string) error
}

type FileBackend struct {
	nfsRoot    string
	nfsAddress string
	bootRoot   string
}

func newFileBackend(nfsAddress, nfsRoot, bootRoot string) (fileBackend, error) {
	if nfsAddress == "" || nfsRoot == "" {
		return &FileBackend{}, fmt.Errorf("fileBackend: nfsAddress or nfsRoot not configured")
	}

	return &FileBackend{
		nfsRoot:    nfsRoot,
		nfsAddress: nfsAddress,
		bootRoot:   bootRoot,
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

func (f *FileBackend) CopyFolder(s, d string) error {
	_, err := exec.Command("rsync", "-xa", s, d).CombinedOutput()
	if err != nil {
		return err
	}

	return nil
}
