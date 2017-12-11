package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func main() {
	httpPort := 8080

	bakeryRoot := os.Getenv("BAKERY_ROOT")
	nfsServer := os.Getenv("NFS_ADDRESS")

	if bakeryRoot == "" {
		panic("BAKERY_ROOT env var not set")
	}

	if nfsServer == "" {
		panic("NFS_ADDRESS env var not set")
	}

	nfsRoot := bakeryRoot + "/nfs/"
	imageFolder := bakeryRoot + "/bakeforms/"
	bootFolder := bakeryRoot + "/boot/"
	mountRoot := bakeryRoot + "/mnt"

	initFolders(nfsRoot, imageFolder, bootFolder, mountRoot)

	nfs, err := newFileBackend(nfsServer, nfsRoot, bootFolder)
	if err != nil {
		panic(err)
	}

	bakeforms, err := newBakeformInventory(imageFolder, mountRoot, nfs)
	if err != nil {
		panic(err.Error())
	}
	defer bakeforms.UnmountAll()

	pile, err := newPiInventory(bakeforms)
	if err != nil {
		panic(err.Error())
	}

	fs, err := newFileServer(nfs, pile)
	if err != nil {
		panic(err.Error())
	}

	r := mux.NewRouter()
	r.Path("/api/v1/files/{piId}/{filename}").Methods(http.MethodGet).HandlerFunc(fs.fileHandler) //Generates files for net booting

	r.Path("/api/v1/fridge").Methods(http.MethodGet).HandlerFunc(pile.FridgeHandler)
	r.Path("/api/v1/fridge").Methods(http.MethodPost).HandlerFunc(pile.BakeHandler)

	//r.HandleFunc("/api/v1/oven/{piId}/reboot", rebootHandler) //Reboots the pi
	r.Path("/api/v1/oven/{piId}").Methods(http.MethodGet).HandlerFunc(pile.GetPiHandler)
	r.Path("/api/v1/oven/{piId}").Methods(http.MethodDelete).HandlerFunc(pile.UnbakeHandler)
	r.Path("/api/v1/oven").Methods(http.MethodGet).HandlerFunc(pile.OvenHandler)

	r.Path("/api/v1/bakeforms").Methods(http.MethodGet).HandlerFunc(bakeforms.ListHandler)
	r.Path("/api/v1/bakeforms/{name}").Methods(http.MethodPost).HandlerFunc(bakeforms.UploadHandler)
	r.Path("/api/v1/bakeforms/{name}").Methods(http.MethodDelete).HandlerFunc(bakeforms.DeleteHandler)

	fmt.Println("Ready to bake!")
	http.ListenAndServe(fmt.Sprintf(":%v", httpPort), r)
}
