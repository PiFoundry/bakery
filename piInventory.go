package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	"database/sql"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

type piInventory interface {
	NewPi(string) piInfo
	GetPi(piId string) (piInfo, error)
	ListFridge() (piList, error)
	ListOven() (piList, error)
	BakeHandler(http.ResponseWriter, *http.Request)
	UnbakeHandler(http.ResponseWriter, *http.Request)
	GetPiHandler(w http.ResponseWriter, r *http.Request)
	OvenHandler(w http.ResponseWriter, r *http.Request)
	FridgeHandler(w http.ResponseWriter, r *http.Request)
	RebootHandler(w http.ResponseWriter, r *http.Request)
}

type PiInventory struct {
	db        *sql.DB
	bakeforms bakeformInventory
}

type bakeRequest struct {
	BakeformName string `json:"bakeformName"`
}

var piProvisionMutexes map[string]*sync.Mutex

func newPiInventory(bakeforms bakeformInventory) (piInventory, error) {
	db, err := sql.Open("sqlite3", "piInventory.db")
	sqlStmt := "create table if not exists inventory (id text not null primary key, status integer, bakeform text);"

	_, err = db.Exec(sqlStmt)
	if err != nil {
		return &PiInventory{}, err
	}

	newInv := &PiInventory{
		db:        db,
		bakeforms: bakeforms,
	}

	stuckPis, _ := newInv.listPis(PREPARING)
	fmt.Println("unstucking Pis that are stuck in the PREPARING state.")
	for _, pi := range stuckPis {
		err := pi.Unstuck()
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	piProvisionMutexes = make(map[string]*sync.Mutex)
	return newInv, nil
}

//NewPi just returns a new piInfo struct. It does not register the info in the DB. Use piInfo.Save() to do so.
func (i *PiInventory) NewPi(piId string) piInfo {
	return &PiInfo{
		db:              i.db,
		parentInventory: i,
		Id:              piId,
		Status:          NOTINUSE,
	}
}

//GetPi finds the pi in the DB. If the pi is not found an empty piInfo struct and an error is returned
func (i *PiInventory) GetPi(piId string) (piInfo, error) {
	rows, err := i.db.Query(fmt.Sprintf("select id, status, bakeform from inventory where id = '%v'", piId))
	if err != nil {
		return &PiInfo{}, err
	}
	defer rows.Close()

	if rows.Next() {
		var id, bakeform string
		var status piStatus

		rows.Scan(&id, &status, &bakeform)

		return &PiInfo{
			db:              i.db,
			parentInventory: i,
			Id:              id,
			Status:          status,
			SourceBakeform:  i.bakeforms.List()[bakeform],
		}, nil
	}

	return &PiInfo{}, fmt.Errorf("%v not found in inventory", piId)
}

func (i *PiInventory) ListFridge() (piList, error) {
	return i.listPis(NOTINUSE)
}

func (i *PiInventory) ListOven() (piList, error) {
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

func (i *PiInventory) listPis(qStatus piStatus) (piList, error) {
	list := make(piList)

	rows, err := i.db.Query(fmt.Sprintf("select id, status, bakeform from inventory where status = %v", qStatus))
	if err != nil {
		return list, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, bakeform string
		var status piStatus

		rows.Scan(&id, &status, &bakeform)

		list[id] = &PiInfo{
			db:              i.db,
			parentInventory: i,
			Id:              id,
			Status:          status,
			SourceBakeform:  i.bakeforms.List()[bakeform],
		}
	}

	return list, nil
}

func (i *PiInventory) BakeHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	//parse the post body
	var params bakeRequest
	body, _ := ioutil.ReadAll(r.Body)
	err := json.Unmarshal(body, &params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	sourceBakeform := i.bakeforms.List()
	useBakeForm, exists := sourceBakeform[params.BakeformName]
	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("bakeform with name %v does not exist", params.BakeformName)))
		return
	}

	//get list of pis in the fridge
	list, err := i.ListFridge()
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
	go targetPi.Bake(useBakeForm)
}

func (i *PiInventory) UnbakeHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	piId := urlvars["piId"]

	pi, err := i.GetPi(piId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if pi.GetStatus() != INUSE {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Can only unbake pis that are actually baked"))
		return
	}

	go pi.Unbake()
	w.WriteHeader(http.StatusOK)
}

func (i *PiInventory) GetPiHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	piId := urlvars["piId"]

	pi, err := i.GetPi(piId)
	if err == nil {
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(pi)
		w.Write(jsonBytes)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (i *PiInventory) OvenHandler(w http.ResponseWriter, r *http.Request) {
	piList, err := i.ListOven()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var jsonBytes []byte
	jsonBytes, err = json.Marshal(piList)
	w.Write(jsonBytes)
}

func (i *PiInventory) FridgeHandler(w http.ResponseWriter, r *http.Request) {
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

func (i *PiInventory) RebootHandler(w http.ResponseWriter, r *http.Request) {
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
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
}
