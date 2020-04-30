// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var rex = regexp.MustCompile("\\A[\\w]+\\s\\w+=\"([\\w:/.]+)\",\\w+=\"([\\w:/.]+)\",\\w+=\"([\\w:/.]+)\"")

const dockerHubURL string = "registry.hub.docker.com"
const dockerHubApiVer string = "v2"

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]
	dir, _ := os.Getwd()
	const tempDir string = "/rootDir"
	executable := command[strings.LastIndex(command, "/")+1 : len(command)]
	workDir := dir + tempDir

	// Create output folder
	var folderMode uint32 = 0o755
	if err := syscall.Mkdir(workDir, folderMode); err != nil {
		log.Fatalf(err)
		os.Exit(1)
	}

	// Create /dev/null
	if err := syscall.Mkdir(workDir+"/dev", folderMode); err != nil {
		log.Fatalf(err)
		os.Exit(1)
	}
	if err := exec.Command("mknod", "-m", "666", workDir+"/dev/null", "c", "1", "3").Run(); err != nil {
		log.Fatalf(err)
		os.Exit(1)
	}

	// Try to retrieve manifest
	var authRealm, authService, authScope string
	var dockerHubUrl string = "https://" + dockerHubURL + "/" + dockerHubApiVer + "/library/"
	resp, err := http.Get(dockerHubUrl + image + "/manifests/latest")
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}
	defer resp.Body.Close()
	respContent, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode == 401 {
		headers := resp.Header
		authHeader := headers.Get("Www-Authenticate")
		data := rex.FindAllStringSubmatch(authHeader, -1)
		authRealm = data[0][1]
		authService = data[0][2]
		authScope = data[0][3]
	}

	// Authenticate
	httpClient := http.Client{}
	req, err := http.NewRequest("GET", authRealm, nil)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	q := req.URL.Query()
	q.Add("service", authService)
	q.Add("scope", authScope)
	req.URL.RawQuery = q.Encode()
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}

	var token AuthToken
	defer resp.Body.Close()
	respContent, err = ioutil.ReadAll(resp.Body)
	json.Unmarshal(respContent, &token)

	// Retrieve image's manifest
	req, err = http.NewRequest("GET", dockerHubUrl+image+"/manifests/latest", nil)
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}

	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}

	var imageManifest ImageManifest
	defer resp.Body.Close()
	respContent, err = ioutil.ReadAll(resp.Body)
	json.Unmarshal(respContent, &imageManifest)

	// Pull layers
	for _, layer := range imageManifest.FsLayers {
		req, err = http.NewRequest("GET", dockerHubUrl+image+"/blobs/"+layer.BlobSum, nil)
		if err != nil {
			log.Fatalf("Error occured, failed to retrieve layer: %v\n", err)
		}

		req.Header.Add("Authorization", "Bearer "+token.AccessToken)
		resp, err = httpClient.Do(req)
		if err != nil {
			log.Fatalf("Error occured, failed to retrieve layer: %v\n", err)
		}

		defer resp.Body.Close()

		// Unpack layers
		respContent, err = ioutil.ReadAll(resp.Body)
		ioutil.WriteFile("layer.tar.gz", respContent, 0755)
		cmd := exec.Command("tar", "-xvzf", "layer.tar.gz", "-C", workDir)
		if err := cmd.Run(); err != nil {
			fmt.Println(err)
		}
	}

	os.Chmod(workDir+"/"+executable, 0755)
	if err := syscall.Chdir(workDir); err != nil {
		log.Fatalf(err)
		os.Exit(1)
	}

	// Chroot to create jail and isolate
	if err := syscall.Chroot(workDir); err != nil {
		log.Fatalf(err)
		os.Exit(1)
	}

	// Create process with PID1
	cmd := exec.Command(executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}
	var exitCode int
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	} else {
		exitCode = 0
	}
	os.Exit(exitCode)
}

type ImageManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Name          string `json:"name"`
	Tag           string `json:"tag"`
	Architecture  string `json:"architecture"`
	FsLayers      []struct {
		BlobSum string `json:"blobSum"`
	} `json:"fsLayers"`
	History []struct {
		V1Compatibility time.Time `json:"v1Compatibility"`
	} `json:"history"`
	Signatures []struct {
		Header struct {
			Jwk struct {
				Crv string `json:"crv"`
				Kid string `json:"kid"`
				Kty string `json:"kty"`
				X   string `json:"x"`
				Y   string `json:"y"`
			} `json:"jwk"`
			Alg string `json:"alg"`
		} `json:"header"`
		Signature string `json:"signature"`
		Protected string `json:"protected"`
	} `json:"signatures"`
}

type AuthToken struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}
