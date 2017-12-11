package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type bakeform interface {
	GetName() string
	GetBootLocation() string
	GenerateRootLocation(pi piInfo) string
	Delete() error
	mount() error
	unmount() error
	Deploy(pi piInfo) error
}

type Bakeform struct {
	Name         string `json:"name"`
	Location     string `json:"location"`
	mountRoot    string
	nfs          fileBackend
	bootLocation string
	MountedOn    []string `json:"mountedOn,omitempty"`
}

type BakeformList map[string]bakeform

func (b *Bakeform) GetName() string {
	return b.Name
}

func (b *Bakeform) GetBootLocation() string {
	return b.bootLocation
}

func (b *Bakeform) Delete() error {
	b.unmount()
	return os.Remove(b.Location)
}

func (b *Bakeform) mount() error {
	if len(b.MountedOn) >= 2 {
		return nil
	}

	//raspbian images have 2 partitions. before mounting we need to map them to devices
	out, err := exec.Command("kpartx", "-av", b.Location).CombinedOutput()
	if err != nil {
		return err
	}

	re, err := regexp.Compile("loop\\d+p\\d+")
	if err != nil {
		return err
	}
	loops := re.FindAll(out, 2)
	if len(loops) < 2 {
		return fmt.Errorf("Image could not be mapped")
	}

	//Go is too fast :). mounting directly after mapping fails. 1s delay fixes it.
	time.Sleep(1 * time.Second)
	//TODO: check for map device to be available instead of fixed sleep

	//Loop through the partitions and mount each one
	for i, loop := range loops {
		loopDevice := "/dev/mapper/" + string(loop)
		mountTarget := fmt.Sprintf("%v/%v-%v", b.mountRoot, b.Name, i)

		err = os.Mkdir(mountTarget, 0777) //create the mount point
		if err != nil {
			exists, _ := regexp.MatchString("file exists$", err.Error())
			if !exists {
				return err
			}
		}

		fmt.Printf("Mounting %v on %v\n", loopDevice, mountTarget)
		err = syscall.Mount(loopDevice, mountTarget, "vfat", 0, "")
		if err != nil && err.Error() != "device or resource busy" { //already mounted if this error occurs. Just continue :){
			err = syscall.Mount(loopDevice, mountTarget, "ext4", 0, "")
			if err != nil && err.Error() != "device or resource busy" { //already mounted if this error occurs. Just continue :)
				return err
			}
		}

		b.MountedOn = append(b.MountedOn, mountTarget) //store the mountpoint
	}

	return nil
}

func (b *Bakeform) unmount() error {
	for i, mountTarget := range b.MountedOn {
		fmt.Println("Unounting: " + mountTarget)
		err := syscall.Unmount(mountTarget, 0)
		if err != nil {
			return err
		}
		b.MountedOn = append(b.MountedOn[:i], b.MountedOn[i+1:]...) //delete the mount point from the slice
	}

	//unmap devices
	_, err := exec.Command("kpartx", "-d", b.Location).Output()
	if err != nil {
		return err
	}

	return nil
}

func (b *Bakeform) GenerateRootLocation(pi piInfo) string {
	rootLoc := b.nfs.GetNfsRoot() + "/" + pi.GetId()
	return strings.Replace(rootLoc, "//", "/", -1)
}

func (b *Bakeform) GenerateBootLocation() string {
	bootLoc := b.nfs.GetBootRoot() + "/" + b.Name
	return strings.Replace(bootLoc, "//", "/", -1)
}

func (b *Bakeform) Deploy(pi piInfo) error {
	err := b.mount()
	if err != nil {
		return err
	}

	//copy root fs
	rootDest := b.GenerateRootLocation(pi)
	source := b.MountedOn[1] + "/"

	fmt.Printf("Cloning bakeform %v to %v\n", b.Name, rootDest)
	err = b.nfs.CopyFolder(source, rootDest)
	if err != nil {
		return err
	}
	///

	//copy boot volume
	// check if exists first
	if b.bootLocation == "" {
		bootDest := b.GenerateBootLocation()
		b.nfs.CopyFolder(b.MountedOn[0]+"/", bootDest)
		b.bootLocation = bootDest
	}
	///

	return regenNfsExports(pi.GetParentInventory())
}

func (b *Bakeform) Undeploy(pi piInfo) error {
	return nil
}
