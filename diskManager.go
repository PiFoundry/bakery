package main

import (
	"encoding/json"
	"net/http"
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
		}
	}

	return dm, nil
}

func (dm *diskManager) RegisterDisk(id, location string) *disk {
	dm.Disks[id] = &disk{
		ID:       id,
		Location: location,
	}

	return dm.Disks[id]
}

func (dm *diskManager) NewDisk() (*disk, error) {
	id := "disk-" + uuid.New().String()
	location, err := dm.fb.CreateNfsFolder(id)
	if err != nil {
		return &disk{}, err
	}

	return dm.RegisterDisk(id, location), nil
}

func (dm *diskManager) DestroyDisk(id string) error {
	/*	_, err := uuid.FromBytes([]byte(strings.Replace(id, "disk-", "", 1)))
		if !strings.HasPrefix(id, "disk-") || err != nil {
			return fmt.Errorf("No valid disk ID given")
		}*/
	delete(dm.Disks, id)
	return dm.fb.DeleteNfsFolder(id)
}

func (dm *diskManager) createDiskHandler(w http.ResponseWriter, r *http.Request) {
	disk, err := dm.NewDisk()
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
