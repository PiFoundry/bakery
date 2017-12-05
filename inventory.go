package main

import (
	"fmt"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type piInventory interface {
	NewPi(string) piInfo
	GetPi(piId string) (piInfo, error)
}

type PiInventory struct {
	db *sql.DB
}

func newInventory() (piInventory, error) {
	db, err := sql.Open("sqlite3", "piInventory.db")
	sqlStmt := "create table if not exists inventory (id text not null primary key, status integer);"

	_, err = db.Exec(sqlStmt)
	if err != nil {
		//fmt.Println(err.Error())
		return &PiInventory{}, err
	}

	return &PiInventory{
		db: db,
	}, nil
}

//NewPi just returns a new piInfo struct. It does not register the info in the DB. Use piInfo.Update() to do so.
func (i *PiInventory) NewPi(piId string) piInfo {
	return &PiInfo{
		db:     i.db,
		Id:     piId,
		Status: NOTINUSE,
	}
}

//GetPi finds the pi in the DB. If the pi is not found an empty piInfo struct and an error is returned
func (i *PiInventory) GetPi(piId string) (piInfo, error) {
	rows, err := i.db.Query(fmt.Sprintf("select id, status from inventory where id = '%v'", piId))
	if err != nil {
		return &PiInfo{}, err
	}
	defer rows.Close()

	if rows.Next() {
		var id string
		var status piStatus
		rows.Scan(&id, &status)

		return &PiInfo{
			db:     i.db,
			Id:     id,
			Status: status,
		}, nil
	}

	return &PiInfo{}, fmt.Errorf("%v not found in inventory", piId)
}

type piStatus int

const (
	NOTINUSE piStatus = 1
	INUSE    piStatus = 2
)

type piInfo interface {
	Update() error
}

type PiInfo struct {
	db     *sql.DB
	Id     string
	Status piStatus
}

func (p *PiInfo) Update() error {
	fmt.Printf("Updating Pi with id: %v to status: %v\n", p.Id, p.Status)
	_, err := p.db.Exec(fmt.Sprintf("insert into inventory(id, status) values('%v', %v)", p.Id, p.Status))
	if err != nil {
		return err
	}

	return nil
}
