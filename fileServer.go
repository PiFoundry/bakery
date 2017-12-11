package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

type fileServer interface {
	fileHandler(http.ResponseWriter, *http.Request)
}

type FileServer struct {
	nfs         fileBackend
	piInventory piInventory
}

type templatevars struct {
	PiId      string
	NfsServer string
	NfsRoot   string
}

func newFileServer(nfs fileBackend, inventory piInventory) (fileServer, error) {
	return &FileServer{
		nfs:         nfs,
		piInventory: inventory,
	}, nil
}

func (f *FileServer) fileHandler(w http.ResponseWriter, r *http.Request) {
	urlvars := mux.Vars(r)
	filename := urlvars["filename"]
	piId := urlvars["piId"]

	//check if piId is allready registered. If not then register.
	pi, err := f.piInventory.GetPi(piId)
	if err != nil {
		fmt.Println("Pi not found in inventory. Putting a new one in the fridge.")
		pi = f.piInventory.NewPi(piId)
		err = pi.Save()
		if err != nil {
			panic(err)
		}
	}

	if pi.GetStatus() == NOTINUSE {
		//Pi is not in inventory or not in use. Then don't server files
		w.WriteHeader(http.StatusNotFound)
		return
	}

	//if filename == cmdline.txt then parse the template. else just serve the file
	bootLocation := pi.GetSourceBakeform().GetBootLocation()
	strings.Replace(bootLocation, "/", "", 1) //remove the first /
	fullFilename := bootLocation + "/" + filename

	if filename != "cmdline.txt" {
		http.ServeFile(w, r, fullFilename)
		return
	} else {

		c := templatevars{
			NfsServer: f.nfs.GetNfsAddress(),
			NfsRoot:   pi.GetRootLocation(),
		}

		fmt.Printf("%v requested for: %v\n", filename, pi.GetId())
		t, err := template.New("templatefile").ParseFiles(fullFilename)
		if err != nil {
			panic(err)
		}
		t.ExecuteTemplate(w, filename, c)
	}
}