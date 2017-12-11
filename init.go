package main

import "os"

func initFolders(folders ...string) {
	for _, folder := range folders {
		os.Mkdir(folder, 0777)
	}

}
