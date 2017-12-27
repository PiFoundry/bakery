package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type diskManager struct {
	Disks map[string]*disk `json:"disks"`
	fb    fileBackend
}

type disk struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Size     int64  `json:"size"`
}

func NewDiskManager(fb fileBackend) (*diskManager, error) {
	dm := &diskManager{
		Disks: make(map[string]*disk),
		fb:    fb,
	}

	diskFolders := fb.GetNfsFolders("*")
	for _, diskFolder := range diskFolders {
		nameParts := strings.Split(diskFolder, "/")
		id := nameParts[len(nameParts)-1]

		dm.Disks[id] = &disk{
			ID:       id,
			Location: diskFolder,
			Size:     dm.getDiskSize(diskFolder),
		}
	}

	return dm, nil
}

func (dm *diskManager) getDiskSize(location string) int64 {
	var size int64 = 0
	f, err := os.Open(path.Join(location, "disk.img"))
	if err == nil {
		fs, err := f.Stat()
		if err == nil {
			size = fs.Size() / 1024 / 1024
		}
	}

	return size
}

func (dm *diskManager) RegisterDisk(id, location string) *disk {
	dm.Disks[id] = &disk{
		ID:       id,
		Location: location,
		Size:     dm.getDiskSize(location),
	}

	return dm.Disks[id]
}

func (dm *diskManager) NewDisk(size int) (*disk, error) {
	if size <= 0 {
		return nil, fmt.Errorf("Disk size should be larger than 0.")
	}

	id := uuid.New().String()
	location, err := dm.fb.CreateNfsFolder(id)
	if err != nil {
		return &disk{}, err
	}

	//Create disk file
	sizeInBytes := int64(size * 1024 * 1024)
	fd, err := os.Create(path.Join(location, "disk.img"))
	defer fd.Close()
	if err != nil {
		return nil, fmt.Errorf("Failed to create output")
	}

	fd.Seek(sizeInBytes-1, 0)
	_, err = fd.Write([]byte{0})
	if err != nil {
		return nil, fmt.Errorf("Write failed")
	}

	return dm.RegisterDisk(id, location), nil
}

func (dm *diskManager) DiskFromBakeform(bf *Bakeform) (*disk, error) {
	err := bf.mount()
	if err != nil {
		return &disk{}, err
	}

	id := uuid.New().String()
	location, err := dm.fb.CopyNfsFolder(bf.MountedOn[1]+"/", id)
	if err != nil {
		return &disk{}, err
	}

	return dm.RegisterDisk(id, location), nil
}

func (dm *diskManager) DestroyDisk(id string) error {
	//TODO check if realy destroying a diks, not something else.... check if id == uuid fe
	delete(dm.Disks, id)
	return dm.fb.DeleteNfsFolder(id)
}

func (dm *diskManager) PutFileOnDisk(diskId, filePath string, content []byte) error {
	disk, exists := dm.Disks[diskId]
	if !exists {
		return fmt.Errorf("Disk with id %v not found", diskId)
	}

	file := path.Join(disk.ID, filePath)
	dm.fb.PutFileInNfsFolder(file, content)

	return nil
}

func (dm *diskManager) createDiskHandler(w http.ResponseWriter, r *http.Request) {
	var params struct {
		Size int `json:"size"`
	}

	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	disk, err := dm.NewDisk(params.Size)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(([]byte(err.Error())))
		return
	}

	jsonBytes, err := json.Marshal(disk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(([]byte(err.Error())))
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(jsonBytes)
}

func (dm *diskManager) destroyDiskHandler(w http.ResponseWriter, r *http.Request) {
	diskId := mux.Vars(r)["diskId"]
	if diskId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := dm.DestroyDisk(diskId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(([]byte(err.Error())))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (dm *diskManager) listDisksHandler(w http.ResponseWriter, r *http.Request) {
	jsonBytes, err := json.Marshal(dm)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(jsonBytes)
}

func (dm *diskManager) getDiskHandler(w http.ResponseWriter, r *http.Request) {
	diskId := mux.Vars(r)["diskId"]
	if diskId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	disk, exists := dm.Disks[diskId]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	jsonBytes, err := json.Marshal(disk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(jsonBytes)
}
