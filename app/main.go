// +build linux

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]           //"/usr/local/bin/docker-explorer" //os.Args[3]
	args := os.Args[4:len(os.Args)] //[]string{"echo", "hey"}             //os.Args[4:len(os.Args)]
	dir, _ := os.Getwd()
	const tempDir string = "/rootDir"
	executable := command[strings.LastIndex(command, "/")+1 : len(command)]
	workDir := dir + tempDir
	var folderMode uint32 = 0o700
	if err := syscall.Mkdir(workDir, folderMode); err != nil {
		//fmt.Println(err)
	}
	if err := syscall.Mkdir(workDir+"/dev", folderMode); err != nil {
		// fmt.Println(err)
	}
	if err := exec.Command("mknod", "-m", "666", workDir+"/dev/null", "c", "1", "3").Run(); err != nil {
		// fmt.Println(err)
	}

	copy(command, workDir+"/"+executable)
	os.Chmod(workDir+"/"+executable, 0100)
	if err := syscall.Chdir(workDir); err != nil {
		// fmt.Println(err)
	}

	if err := syscall.Chroot(workDir); err != nil {
		// fmt.Println(err)
	}

	cmd := exec.Command("/"+executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}
	var exitCode int
	if err := cmd.Run(); err != nil {
		// fmt.Println(err)1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	} else {
		exitCode = 0
	}
	os.Exit(exitCode)
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		// return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func ls(dir string) {
	files, err := ioutil.ReadDir("./" + dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		fmt.Println(f.Name())
	}
}
