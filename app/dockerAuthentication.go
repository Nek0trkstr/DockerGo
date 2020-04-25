package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

var rex = regexp.MustCompile("\\A[\\w]+\\s\\w+=\"([\\w:/.]+)\",\\w+=\"([\\w:/.]+)\",\\w+=\"([\\w:/.]+)\"")

const dockerHubURL string = "registry.hub.docker.com"
const dockerHubApiVer string = "v2"

type AuthToken struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

func main() {
	var dockerHubUrl string = "https://" + dockerHubURL + "/" + dockerHubApiVer + "/library/"
	resp, err := http.Get(dockerHubUrl + "alpine/manifests/latest")
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}
	defer resp.Body.Close()
	var authRealm, authService, authScope string
	if resp.StatusCode == 401 {
		headers := resp.Header
		authHeader := headers.Values("Www-Authenticate")[0]
		data := rex.FindAllStringSubmatch(authHeader, -1)
		authRealm = data[0][1]
		authService = data[0][2]
		authScope = data[0][3]
	}

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
	respContent, err := ioutil.ReadAll(resp.Body)
	json.Unmarshal(respContent, &token)

	req, err = http.NewRequest("GET", dockerHubUrl+"alpine/manifests/latest", nil)
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}

	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatalf("Error occured, failed to retrieve manifest: %v\n", err)
	}

	defer resp.Body.Close()
	respContent, err = ioutil.ReadAll(resp.Body)
	fmt.Printf(string(respContent))
}
