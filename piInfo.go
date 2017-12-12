package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"time"
)

type piStatus int

const (
	NOTINUSE  piStatus = 1
	INUSE     piStatus = 2
	PREPARING piStatus = 3
)

type piList map[string]piInfo

type piInfo interface {
	GetId() string
	GetRootLocation() string
	GetParentInventory() piInventory
	Save() error
	Bake(bakeform) error
	Unbake() error
	GetStatus() piStatus
	GetSourceBakeform() bakeform
	Unstuck() error
	PowerOn() error
	PowerOff() error
	PowerCycle() error
}

type PiInfo struct {
	db              *sql.DB
	parentInventory piInventory
	Id              string   `json:"id"`
	Status          piStatus `json:"status"`
	SourceBakeform  bakeform `json:"sourceBakeform,omitempty"`
	provisionMutex  *sync.Mutex
}

func (p *PiInfo) GetId() string {
	return p.Id
}

func (p *PiInfo) GetParentInventory() piInventory {
	return p.parentInventory
}

func (p *PiInfo) GetRootLocation() string {
	return p.SourceBakeform.GenerateRootLocation(p)
}

func (p *PiInfo) Save() error {
	//fmt.Printf("Updating Pi with id: %v to status: %v\n", p.Id, p.Status)
	bakeformString := ""
	if p.SourceBakeform != nil {
		bakeformString = p.SourceBakeform.GetName()
	}
	_, err := p.db.Exec(fmt.Sprintf("insert into inventory(id, status, bakeform) values('%v', %v, '%v')", p.Id, p.Status, bakeformString))
	if err != nil {
		if err.Error() == "UNIQUE constraint failed: inventory.id" {
			stmt := fmt.Sprintf("update inventory set status = %v, bakeform = '%v' where id = '%v'", p.Status, bakeformString, p.Id)
			_, err := p.db.Exec(stmt)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func (p *PiInfo) Bake(image bakeform) error {
	p.provisionMutex.Lock()
	defer p.provisionMutex.Unlock()
	fmt.Printf("Baking pi with id: %v\n", p.Id)
	//Set state to baking and Store State
	p.Status = PREPARING
	p.SourceBakeform = image
	err := p.Save()
	if err != nil {
		return err
	}

	//Create and fill system NFS export
	err = image.Deploy(p)
	if err != nil {
		fmt.Println(err.Error())
		p.Status = NOTINUSE
		p.SourceBakeform = nil
		p.Save()
		return err
	}

	fmt.Printf("Pi with id %v is ready!\n", p.Id)

	//Set state to INUSE and Store State
	p.Status = INUSE
	err = p.Save()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	err = p.PowerCycle()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	return nil
}

func (p *PiInfo) Unbake() error {
	p.provisionMutex.Lock()
	defer p.provisionMutex.Unlock()

	fmt.Printf("Unbaking pi with id: %v\n", p.Id)
	err := p.PowerOff()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	//Get root location before deleting the pi
	rootLoc := p.GetRootLocation()

	//Set state to NOTINUSE and Store State
	p.Status = NOTINUSE
	p.SourceBakeform = nil
	err = p.Save()
	if err != nil {
		return err
	}

	regenNfsExports(p.parentInventory)
	err = os.RemoveAll(rootLoc)
	if err != nil {
		fmt.Println(err.Error())
	}

	return err
}

func (p *PiInfo) GetStatus() piStatus {
	return p.Status
}

func (p *PiInfo) GetSourceBakeform() bakeform {
	return p.SourceBakeform
}

func (p *PiInfo) Unstuck() error {
	p.Status = NOTINUSE
	return p.Save()
}

func (p *PiInfo) doPpiAction(action string) error {
	if action != "poweron" && action != "poweroff" {
		return fmt.Errorf("action %v not supported", action)
	}

	params := ppiParams{
		PiId:   p.Id,
		Action: action,
	}

	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return err
	}

	ppicmd := exec.Command("./ppi")
	ppistdin, err := ppicmd.StdinPipe()
	if err != nil {
		return err
	}

	ppistdout, _ := ppicmd.StdoutPipe()
	ppistderr, _ := ppicmd.StderrPipe()

	err = ppicmd.Start()
	if err != nil {
		return err
	}

	io.WriteString(ppistdin, string(jsonBytes))
	ppistdin.Close()

	out, _ := ioutil.ReadAll(ppistdout)
	outerr, _ := ioutil.ReadAll(ppistderr)

	err = ppicmd.Wait()
	if err != nil || len(outerr) != 0 || string(out) != "ok" {
		//fmt.Printf("ppi output: %v/%v", string(outerr), string(out))
		return fmt.Errorf("%v %v", string(outerr), string(out))
	}

	return nil
}

func (p *PiInfo) PowerOn() error {
	return p.doPpiAction("poweron")
}

func (p *PiInfo) PowerOff() error {
	return p.doPpiAction("poweroff")
}

func (p *PiInfo) PowerCycle() error {
	err := p.doPpiAction("poweroff")
	if err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	return p.doPpiAction("poweron")
}
