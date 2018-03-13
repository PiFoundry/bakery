package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"

	"database/sql"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

type piManager interface {
	NewPi(string) PiInfo
	GetPi(piId string) (PiInfo, error)
	ListFridge() (piList, error)
	ListOven() (piList, error)
	BakePi(PiInfo, *Bakeform)
	BakeHandler(http.ResponseWriter, *http.Request)
	UnbakeHandler(http.ResponseWriter, *http.Request)
	GetPiHandler(w http.ResponseWriter, r *http.Request)
	OvenHandler(w http.ResponseWriter, r *http.Request)
	FridgeHandler(w http.ResponseWriter, r *http.Request)
	RebootHandler(w http.ResponseWriter, r *http.Request)
	AttachDiskHandler(w http.ResponseWriter, r *http.Request)
	DetachDiskHandler(w http.ResponseWriter, r *http.Request)
	UploadHandler(w http.ResponseWriter, r *http.Request)
	DownloadHandler(w http.ResponseWriter, r *http.Request)
}

type PiManager struct {
	db                 *sql.DB
	bakeforms          bakeformInventory
	diskManager        *diskManager
	piProvisionMutexes map[string]*sync.Mutex
	ppiPath            string
}

type bakeRequest struct {
	BakeformName string `json:"bakeformName"`
}

func NewPiManager(bakeforms bakeformInventory, dm *diskManager, inventoryDbPath, ppiPath string) (piManager, error) {
	db, err := sql.Open("sqlite3", inventoryDbPath)
	sqlStmt := "create table if not exists inventory (id text not null primary key, status integer, bakeform text, diskIds text);"

	_, err = db.Exec(sqlStmt)
	if err != nil {
		return &PiManager{}, err
	}

	newInv := &PiManager{
		db:                 db,
		bakeforms:          bakeforms,
		piProvisionMutexes: make(map[string]*sync.Mutex),
		diskManager:        dm,
	}

	stuckPis, _ := newInv.listPis(PREPARING)
	log.Println("unstucking Pis that are stuck in the PREPARING state.")
	for _, pi := range stuckPis {
		pi.Status = NOTINUSE
		err := pi.Save()
		if err != nil {
			log.Println(err.Error())
		}
	}

	return newInv, nil
}

//NewPi just returns a new piInfo struct. It does not register the info in the DB. Use piInfo.Save() to do so.
func (i *PiManager) NewPi(piId string) PiInfo {
	return PiInfo{
		db:      i.db,
		Id:      piId,
		Status:  NOTINUSE,
		ppiPath: i.ppiPath,
	}
}

//GetPi finds the pi in the DB. If the pi is not found an empty piInfo struct and an error is returned
func (i *PiManager) GetPi(piId string) (PiInfo, error) {
	rows, err := i.db.Query(fmt.Sprintf("select id, status, bakeform, diskIds from inventory where id = '%v'", piId))
	if err != nil {
		return PiInfo{}, err
	}
	defer rows.Close()

	if rows.Next() {
		var id, bakeform, diskIdsString string
		var status piStatus

		rows.Scan(&id, &status, &bakeform, &diskIdsString)
		pi := PiInfo{
			db:             i.db,
			Id:             id,
			Status:         status,
			SourceBakeform: i.bakeforms.List()[bakeform],
		}

		diskIds := strings.Split(diskIdsString, ",")
		for _, diskId := range diskIds {
			pi.Disks = append(pi.Disks, i.diskManager.Disks[diskId])
		}

		return pi, nil
	}

	return PiInfo{}, fmt.Errorf("%v not found in inventory", piId)
}

func (i *PiManager) ListFridge() (piList, error) {
	return i.listPis(NOTINUSE)
}

func (i *PiManager) ListOven() (piList, error) {
	inuse, err := i.listPis(INUSE)
	if err == nil {
		preparing, err := i.listPis(PREPARING)

		for k, v := range inuse {
			preparing[k] = v
		}
		return preparing, err
	}

	return nil, err
}

func (pm *PiManager) BakePi(pi PiInfo, bf *Bakeform) {
	if _, exists := pm.piProvisionMutexes[pi.Id]; !exists {
		pm.piProvisionMutexes[pi.Id] = &sync.Mutex{}
	}
	pm.piProvisionMutexes[pi.Id].Lock()
	defer pm.piProvisionMutexes[pi.Id].Unlock()

	log.Printf("Baking pi: %v\n", pi.Id)

	//update the status to PREPARING
	if err := pi.SetStatus(PREPARING); err != nil {
		log.Println(err)
		return
	}

	//Deploy the disk from image
	log.Println("Cloning bakeform...")
	dsk, err := pm.diskManager.DiskFromBakeform(bf)
	if err != nil {
		log.Println(err.Error())
		pi.SetStatus(NOTINUSE)
		return
	}

	//Attach the disk to the pi
	log.Println("Attaching cloned disk")
	err = pi.AttachDisk(dsk)
	if err != nil {
		log.Println(err.Error())
		pm.diskManager.DestroyDisk(dsk.ID)
		pi.SetStatus(NOTINUSE)
		return
	}

	pi.Status = INUSE
	pi.SourceBakeform = bf
	pi.Save()

	log.Printf("Pi with id %v is ready!\n", pi.Id)
}

func (pm *PiManager) UnbakePi(pi *PiInfo, bf *Bakeform) {
	if _, exists := pm.piProvisionMutexes[pi.Id]; !exists {
		pm.piProvisionMutexes[pi.Id] = &sync.Mutex{}
	}
	pm.piProvisionMutexes[pi.Id].Lock()
	defer pm.piProvisionMutexes[pi.Id].Unlock()

	err := pi.Unbake(pm.diskManager)
	if err != nil {
		log.Println(err.Error())
	}
}

func (i *PiManager) listPis(qStatus piStatus) (piList, error) {
	list := make(piList)

	rows, err := i.db.Query(fmt.Sprintf("select id, status, bakeform, diskIds from inventory where status = %v", qStatus))
	if err != nil {
		return list, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, bakeform, diskIdsString string
		var status piStatus

		rows.Scan(&id, &status, &bakeform, &diskIdsString)

		pi := PiInfo{
			db:             i.db,
			Id:             id,
			Status:         status,
			SourceBakeform: i.bakeforms.List()[bakeform],
		}

		diskIds := strings.Split(diskIdsString, ",")
		for _, diskId := range diskIds {
			pi.Disks = append(pi.Disks, i.diskManager.Disks[diskId])
		}

		list[id] = pi
	}

	return list, nil
}

func (pm *PiManager) BakeHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	//parse the post body
	var params bakeRequest
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	sourceBakeform := pm.bakeforms.List()
	useBakeForm, exists := sourceBakeform[params.BakeformName]
	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("bakeform with name %v does not exist", params.BakeformName)))
		return
	}

	//get list of pis in the fridge
	list, err := pm.ListFridge()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	//select one from the list. don't really care which one
	targetPiId := ""
	for key, _ := range list {
		targetPiId = key
		break
	}

	if targetPiId == "" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("{ \"error\": \"no available Pi found\"}"))
		return
	}

	targetPi := list[targetPiId]

	jsonBytes, err := json.Marshal(targetPi)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write(jsonBytes)

	//Start the provisioning process (baking) asynchronously and return the piInfo object for the selected pi
	//the client should check /api/v1/oven/{piId} for the status of the pi
	go pm.BakePi(targetPi, useBakeForm)
}

func (i *PiManager) UnbakeHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	piId := urlvars["piId"]

	pi, err := i.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if pi.Status != INUSE {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Can only unbake pis that are actually baked"))
		return
	}

	go pi.Unbake(i.diskManager)
	w.WriteHeader(http.StatusOK)
}

func (i *PiManager) GetPiHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	piId := urlvars["piId"]

	pi, err := i.GetPi(piId)
	if err == nil {
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(pi)
		w.Write(jsonBytes)
	}

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

func (i *PiManager) OvenHandler(w http.ResponseWriter, r *http.Request) {
	piList, err := i.ListOven()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var jsonBytes []byte
	jsonBytes, err = json.Marshal(piList)
	w.Write(jsonBytes)
}

func (i *PiManager) FridgeHandler(w http.ResponseWriter, r *http.Request) {
	piList, err := i.ListFridge()

	if err == nil {
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(piList)
		w.Write(jsonBytes)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (i *PiManager) RebootHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	piId := params["piId"]

	pi, err := i.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Pi not in inventory"))
		return
	}

	err = pi.PowerCycle()
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (pm *PiManager) AttachDiskHandler(w http.ResponseWriter, r *http.Request) {
	var associateRequest struct {
		DiskId string `json:"diskId"`
	}

	defer r.Body.Close()

	piId := mux.Vars(r)["piId"]
	err := json.NewDecoder(r.Body).Decode(&associateRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error parsing posted data"))
		return
	}

	pi, err := pm.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Pi not found"))
		return
	}

	dsk, exists := pm.diskManager.Disks[associateRequest.DiskId]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Disk not found"))
		return
	}

	err = pi.AttachDisk(dsk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error associating disk"))
		return
	}
}

func (pm *PiManager) DetachDiskHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	piId := params["piId"]
	diskId := params["diskId"]

	pi, err := pm.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Pi not found"))
		return
	}

	dsk, exists := pm.diskManager.Disks[diskId]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Disk not found"))
		return
	}

	err = pi.DetachDisk(dsk)
}

func (pm *PiManager) UploadHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	params := mux.Vars(r)
	piId := params["piId"]
	filename := path.Join("piConfig/", params["filename"])

	pi, err := pm.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Pi not found"))
		return
	}

	content, _ := ioutil.ReadAll(r.Body)

	if pi.Status != INUSE || len(pi.Disks) == 0 {
		w.WriteHeader(http.StatusNotExtended)
		w.Write([]byte("Pi not in ready state"))
	}

	//Lock the povisioning mutex to prevent the pi from disappearing while we put a file
	if _, exists := pm.piProvisionMutexes[pi.Id]; !exists {
		pm.piProvisionMutexes[pi.Id] = &sync.Mutex{}
	}
	pm.piProvisionMutexes[pi.Id].Lock()
	defer pm.piProvisionMutexes[pi.Id].Unlock()

	diskId := pi.Disks[0].ID
	err = pm.diskManager.PutFileOnDisk(diskId, filename, content)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error putting file on disk: " + err.Error()))
	}

	w.WriteHeader(http.StatusOK)
}

func (pm *PiManager) DownloadHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	params := mux.Vars(r)
	piId := params["piId"]
	filename := path.Join("piConfig/", params["filename"])

	pi, err := pm.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Pi not found"))
		return
	}

	diskId := pi.Disks[0].ID

	content, err := pm.diskManager.GetFileFromDisk(diskId, filename)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error getting file from disk: " + err.Error()))
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
}
