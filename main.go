package main

import (
	"fmt"
	"net/http"
	"os"
	"path"

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

	nfsRoot := path.Join(bakeryRoot, "/nfs/")
	imageFolder := path.Join(bakeryRoot, "/bakeforms/")
	bootFolder := path.Join(bakeryRoot, "/boot/")
	mountRoot := path.Join(bakeryRoot, "/mnt")

	initFolders(nfsRoot, imageFolder, bootFolder, mountRoot)

	fb, err := newFileBackend(nfsServer, nfsRoot, bootFolder)
	if err != nil {
		panic(err)
	}

	diskmgr, err := NewDiskManager(fb)

	bakeforms, err := newBakeformInventory(imageFolder, mountRoot, fb)
	if err != nil {
		panic(err.Error())
	}
	defer bakeforms.UnmountAll()

	pile, err := NewPiManager(bakeforms, diskmgr)
	if err != nil {
		panic(err.Error())
	}

	fs, err := newFileServer(fb, pile, diskmgr)
	if err != nil {
		panic(err.Error())
	}

	pis, err := pile.ListOven()
	for _, pi := range pis {
		err = pi.PowerOn()
		if err != nil {
			fmt.Println("Could not restore powerstate of rPi with ID:" + pi.Id)
		}
	}

	r := mux.NewRouter()
	r.Path("/api/v1/files/{piId}/{filename}").Methods(http.MethodGet).HandlerFunc(fs.fileHandler) //Generates files for net booting

	r.Path("/api/v1/fridge").Methods(http.MethodGet).HandlerFunc(pile.FridgeHandler)
	r.Path("/api/v1/fridge").Methods(http.MethodPost).HandlerFunc(pile.BakeHandler)

	r.Path("/api/v1/oven/{piId}/powercycle").Methods(http.MethodPost).HandlerFunc(pile.RebootHandler)
	r.Path("/api/v1/oven/{piId}/disks").Methods(http.MethodPost).HandlerFunc(pile.AttachDiskHandler)
	r.Path("/api/v1/oven/{piId}/disks/{diskId}").Methods(http.MethodDelete).HandlerFunc(pile.DetachDiskHandler)
	r.Path("/api/v1/oven/{piId}/upload/{filename}").Methods(http.MethodPost).HandlerFunc(pile.UploadHandler)
	r.Path("/api/v1/oven/{piId}").Methods(http.MethodGet).HandlerFunc(pile.GetPiHandler)
	r.Path("/api/v1/oven/{piId}").Methods(http.MethodDelete).HandlerFunc(pile.UnbakeHandler)
	r.Path("/api/v1/oven").Methods(http.MethodGet).HandlerFunc(pile.OvenHandler)

	r.Path("/api/v1/bakeforms").Methods(http.MethodGet).HandlerFunc(bakeforms.ListHandler)
	r.Path("/api/v1/bakeforms/{name}").Methods(http.MethodPost).HandlerFunc(bakeforms.UploadHandler)
	r.Path("/api/v1/bakeforms/{name}").Methods(http.MethodDelete).HandlerFunc(bakeforms.DeleteHandler)

	r.Path("/api/v1/disks/{diskId}").Methods(http.MethodDelete).HandlerFunc(diskmgr.destroyDiskHandler)
	r.Path("/api/v1/disks/{diskId}").Methods(http.MethodGet).HandlerFunc(diskmgr.getDiskHandler)
	r.Path("/api/v1/disks").Methods(http.MethodPost).HandlerFunc(diskmgr.createDiskHandler)
	r.Path("/api/v1/disks").Methods(http.MethodGet).HandlerFunc(diskmgr.listDisksHandler)

	fmt.Println("Ready to bake!")
	http.ListenAndServe(fmt.Sprintf(":%v", httpPort), r)
}
