package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type templatevars struct {
	PiId      string
	NfsServer string
	NfsRoot   string
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	piId := r.URL.Query().Get("piId")

	//TODO:check if piId is allready registered. If not then register. If registered then check state and only deliver file if state == inuse
	pi, err := inventory.GetPi(piId)
	if err != nil {
		fmt.Println("Pi not found in inventory. Putting a new one in the oven.")
		pi = inventory.NewPi(piId)
		err = pi.Update()
		if err != nil {
			fmt.Println(err)
			panic(err)
		}
	}

	urlParts := strings.Split(r.URL.EscapedPath(), "/")
	filename := urlParts[len(urlParts)-1]

	c := templatevars{
		PiId:      piId,
		NfsServer: nfsServer,
		NfsRoot:   nfsRoot,
	}

	fmt.Printf("%v requested for: %v\n", filename, c.PiId)

	fullFilename := "fileTemplates/" + filename
	t, err := template.New("templatefile").ParseFiles(fullFilename)
	if err != nil {
		panic(err)
	}
	t.ExecuteTemplate(w, filename, c)
}
