package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
)

type cmdlinevars struct {
	PiId      string
	NfsServer string
	NfsRoot   string
}

var nfsRoot string
var nfsServer string

func cmdlineTxtHandler(w http.ResponseWriter, r *http.Request) {
	piId := r.URL.Query().Get("piId")

	c := cmdlinevars{
		PiId:      piId,
		NfsServer: nfsServer,
		NfsRoot:   nfsRoot,
	}
	//TODO:check if piId is allready registered. If not then register. If registered then check state and only deliver file if state == inuse

	fmt.Printf("cmdline.txt requested for: %v\n", c.PiId)
	t, err := template.New("cmdlinetxt").ParseFiles("cmdline.tmpl")
	if err != nil {
		panic(err)
	}
	t.ExecuteTemplate(w, "cmdline.tmpl", c)
}

func main() {
	httpPort := 8080

	nfsServer = os.Getenv("NFS_SERVER")
	nfsRoot = os.Getenv("NFS_ROOT")

	if nfsServer == "" {
		panic("NFS_SERVER env var not set")
	}

	if nfsRoot == "" {
		panic("NFS_SERVER env var not set")
	}

	http.HandleFunc("/api/v1/files/cmdline.txt", cmdlineTxtHandler)
	http.ListenAndServe(fmt.Sprintf(":%v", httpPort), nil)
}
