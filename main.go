package main

import (
	"fmt"
	"net/http"
	"os"
)

var nfsRoot string
var nfsServer string
var inventory piInventory

func main() {
	httpPort := 8080

	nfsServer = os.Getenv("NFS_SERVER")
	nfsRoot = os.Getenv("NFS_ROOT")

	var err error
	inventory, err = newInventory()
	if err != nil {
		fmt.Println(err.Error())
	}

	if nfsServer == "" {
		panic("NFS_SERVER env var not set")
	}

	if nfsRoot == "" {
		panic("NFS_SERVER env var not set")
	}

	http.HandleFunc("/api/v1/files/cmdline.txt", fileHandler)
	http.ListenAndServe(fmt.Sprintf(":%v", httpPort), nil)
}
