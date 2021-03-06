package main

import (
	"html/template"
	"log"
	"net/http"
	"path"

	"github.com/gorilla/mux"
)

type fileServer interface {
	fileHandler(http.ResponseWriter, *http.Request)
}

type FileServer struct {
	nfs         fileBackend
	piInventory piManager
	diskManager *diskManager
}

type templatevars struct {
	PiId      string
	NfsServer string
	NfsRoot   string
}

func newFileServer(nfs fileBackend, inventory piManager, dm *diskManager) (fileServer, error) {
	return &FileServer{
		nfs:         nfs,
		piInventory: inventory,
		diskManager: dm,
	}, nil
}

func (f *FileServer) fileHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	filename := urlvars["filename"]
	piId := urlvars["piId"]

	//check if piId is allready registered. If not then register.
	pi, err := f.piInventory.GetPi(piId)
	if err != nil {
		log.Println("Pi not found in inventory. Putting a new one in the fridge.")
		pi = f.piInventory.NewPi(piId)
		err = pi.Save()
		if err != nil {
			panic(err)
		}
	}

	if pi.Status == NOTINUSE {
		//Pi is not in inventory or not in use. Then don't serve files and power it off
		log.Printf("Pi %v came online but it's not in use. Powering it off\n", pi.Id)
		err = pi.PowerOff()
		if err != nil {
			log.Println("A Pi just came online but I can't control its power state. Error:" + err.Error())
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	//if filename == cmdline.txt then parse the template. else just serve the file
	bootLocation := pi.SourceBakeform.bootLocation
	//strings.Replace(bootLocation, "/", "", 1) //remove the first /
	fullFilename := path.Join(bootLocation, filename)

	if filename != "cmdline.txt" {
		http.ServeFile(w, r, fullFilename)
		return
	} else {

		c := templatevars{
			NfsServer: f.nfs.GetNfsAddress(),
			NfsRoot:   pi.Disks[0].Location,
		}

		log.Printf("%v requested for: %v\n", filename, pi.Id)
		t, err := template.New("templatefile").ParseFiles(fullFilename)
		if err != nil {
			panic(err)
		}
		t.ExecuteTemplate(w, filename, c)
	}
}
