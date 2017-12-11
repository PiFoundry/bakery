package main

import (
	"database/sql"
	"fmt"
	"os"
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
}

type PiInfo struct {
	db              *sql.DB
	parentInventory piInventory
	Id              string   `json:"id"`
	Status          piStatus `json:"status"`
	SourceBakeform  bakeform `json:"sourceBakeform,omitempty"`
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
		return err
	}

	fmt.Printf("Pi with id %v is ready!\n", p.Id)

	//Set state to INUSE and Store State
	p.Status = INUSE
	return p.Save()
}

func (p *PiInfo) Unbake() error {
	fmt.Printf("Unbaking pi with id: %v\n", p.Id)

	//Get root location before deleting the pi
	rootLoc := p.GetRootLocation()

	//Set state to NOTINUSE and Store State
	p.Status = NOTINUSE
	p.SourceBakeform = nil
	err := p.Save()
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
