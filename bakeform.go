package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"
)

type Bakeform struct {
	Name         string `json:"name"`
	Location     string `json:"location"`
	mountRoot    string
	fb           fileBackend
	bootLocation string
	MountedOn    []string `json:"-"`
}

type BakeformList map[string]*Bakeform

func (b *Bakeform) Delete() error {
	//TODO: Check if not in use

	err := b.unmount()
	if err != nil {
		return fmt.Errorf("Unable to unmount image. %v", err)
	}

	err = os.RemoveAll(b.bootLocation)
	if err != nil {
		log.Printf("Unable to remove bootlocation.Deleting image anyways.")
	}

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

		log.Printf("Mounting %v on %v\n", loopDevice, mountTarget)
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
		log.Println("Unounting: " + mountTarget)
		err := syscall.Unmount(mountTarget, 0)
		if err != nil {
			return err
		}

		b.MountedOn = append(b.MountedOn[:i], b.MountedOn[i+1:]...) //delete the mount point from the slice
		//b.MountedOn[i] = nil
	}

	//unmap devices
	_, err := exec.Command("kpartx", "-d", b.Location).Output()
	if err != nil {
		return err
	}

	return nil
}
