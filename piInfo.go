package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"strings"
	"time"
)

type piStatus int

const (
	NOTINUSE  piStatus = 1
	INUSE     piStatus = 2
	PREPARING piStatus = 3
)

type piList map[string]PiInfo

type PiInfo struct {
	db             *sql.DB
	Id             string    `json:"id"`
	Status         piStatus  `json:"status"`
	Disks          []*disk   `json:"disks,omitempty"`
	SourceBakeform *Bakeform `json:"sourceBakeform,omitempty"`
}

func (p *PiInfo) SetStatus(status piStatus) error {
	p.Status = status
	return p.Save()
}

func (p *PiInfo) Save() error {
	bakeformString := ""
	if p.SourceBakeform != nil {
		bakeformString = p.SourceBakeform.Name
	}

	_, err := p.db.Exec(fmt.Sprintf("insert into inventory(id, status, bakeform, diskIds) values('%v', %v, '%v', '%v')", p.Id, p.Status, bakeformString, ""))
	if err != nil {
		if err.Error() == "UNIQUE constraint failed: inventory.id" {
			var diskIds []string
			for _, diskStruct := range p.Disks {
				if diskStruct != nil {
					diskIds = append(diskIds, diskStruct.ID)
				}
			}

			diskIdsString := strings.Join(diskIds, ",")
			stmt := fmt.Sprintf("update inventory set status = %v, bakeform = '%v', diskIds = '%v' where id = '%v'", p.Status, bakeformString, diskIdsString, p.Id)
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

func (p *PiInfo) Unbake(dm *diskManager) error {
	log.Printf("Unbaking pi with id: %v\n", p.Id)
	err := p.PowerOff()
	if err != nil {
		log.Println(err.Error())
		return err
	}

	//Get disk locations before deleting the pi
	disks := p.Disks

	//Set state to NOTINUSE and Store State
	p.Status = NOTINUSE
	p.SourceBakeform = nil
	err = p.Save()
	if err != nil {
		return err
	}

	//delete attached disks (including root)
	for _, d := range disks {
		log.Println("Destroying disk: " + d.Location)
		err := dm.DestroyDisk(d.ID)
		if err != nil {
			log.Println(err.Error())
		}
	}

	return err
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
		//log.Printf("ppi output: %v/%v", string(outerr), string(out))
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

func (p *PiInfo) AttachDisk(dsk *disk) error {
	p.Disks = append(p.Disks, dsk)
	return p.Save()
}

func (p *PiInfo) DetachDisk(dsk *disk) error {
	var dskIndex int
	for i, d := range p.Disks {
		if d == dsk {
			dskIndex = i
			break
		}
	}

	if dskIndex == 0 {
		return fmt.Errorf("Cannot unassociate boot volume")
	}

	p.Disks = append(p.Disks[:dskIndex], p.Disks[:dskIndex+1]...)
	return p.Save()
}
