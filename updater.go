package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const colorReset string = "\033[0m"
const colorGreen string = "\033[32m"
const colorRed string = "\033[31m"
const colorBlue string = "\033[34m"

type updater struct {
	repository string
	token      string
	directory  string
}

func (u updater) RepositoryURL() string {
	return fmt.Sprintf("https://api.github.com/repos/%s/actions/artifacts", u.repository)
}

func (u updater) AddAuthorizationHeader(req *http.Request) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", u.token))
}

func (u updater) Artifacts() (artifacts, error) {
	var data artifacts
	client := &http.Client{}
	req, err := http.NewRequest("GET", u.RepositoryURL(), nil)
	u.AddAuthorizationHeader(req)

	if err != nil {
		return data, err
	}

	resp, err := client.Do(req)

	if err != nil {
		return data, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return data, fmt.Errorf("received non 200 response code of %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return data, err
	}

	unmarshalErr := json.Unmarshal(body, &data)

	if unmarshalErr != nil {
		return data, unmarshalErr
	}

	return data, nil
}

func (u updater) DownloadFile(URL, fileName string) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", URL, nil)
	u.AddAuthorizationHeader(req)

	if err != nil {
		return err
	}

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("received non 200 response code of %d", resp.StatusCode)
	}

	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func (u updater) DownloadAndReplace(artifact artifact) error {
	sizeValue, sizeSuffix := artifact.Size()

	fmt.Printf("Downloading artifact archive `%s` (%.2f %s) created at %s\n", artifact.Name, sizeValue, sizeSuffix, artifact.CreatedAt)
	fmt.Printf("Artifact location URL: %s\n", artifact.ArchiveDownloadURL)
	fmt.Println("Please be patient ...")
	err := u.DownloadFile(artifact.ArchiveDownloadURL, "dist.zip")

	if err != nil {
		return err
	}

	_, statErr := os.Stat(u.directory)

	if os.IsNotExist(statErr) {
		fmt.Println("Directory doesn't exist, creating one")
		mkdirError := os.Mkdir(u.directory, 0755)

		if mkdirError != nil {
			return mkdirError
		}
	} else {
		fmt.Println("Removing catalog contents")
		files, err := ioutil.ReadDir(u.directory)

		if err != nil {
			return err
		}

		for _, f := range files {
			filePath := path.Join(u.directory, f.Name())

			if f.IsDir() {
				err := os.RemoveAll(filePath)

				if err != nil {
					return err
				}
			} else {
				err := os.Remove(filePath)

				if err != nil {
					return err
				}
			}
		}
	}

	fmt.Println("Extracting archive contents")
	_, unzipErr := unzip("dist.zip", u.directory)

	if unzipErr != nil {
		return unzipErr
	}

	fmt.Println("Removing archive")
	removeErr := os.Remove("dist.zip")

	if removeErr != nil {
		return removeErr
	}

	return nil
}

type artifact struct {
	ID                 int    `json:"id"`
	NodeID             string `json:"node_id"`
	Name               string `json:"name"`
	SizeInBytes        int    `json:"size_in_bytes"`
	URL                string `json:"url"`
	ArchiveDownloadURL string `json:"archive_download_url"`
	Expired            bool   `json:"expired"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	ExpiresAt          string `json:"expires_at"`
}

func (a artifact) Size() (float64, string) {
	if a.SizeInBytes > 1024 * 1024 * 1024 * 1024 {
		return float64(a.SizeInBytes) / float64(1024*1024*1024*1024), "terabytes"
	} else if a.SizeInBytes > 1024 * 1024 * 1024 {
		return float64(a.SizeInBytes / (1024 * 1024 * 1024)), "gigabytes"
	} else if a.SizeInBytes > 1024 * 1024 {
		return float64(a.SizeInBytes) / float64(1024*1024), "megabytes"
	} else if a.SizeInBytes > 1024 {
		return float64(a.SizeInBytes) / float64(1024), "kilobytes"
	}

	return float64(a.SizeInBytes), "bytes"
}

type artifacts struct {
	Count     int        `json:"total_count"`
	Artifacts []artifact `json:"artifacts"`
}

func (a artifacts) HasArtifacts() bool {
	return a.Count > 0
}

func (a artifacts) LatestActiveSherpaArtifact() (artifact, error) {
	var response artifact
	err := errors.New("no suitable artifacts found")

	for _, artifact := range a.Artifacts {
		if artifact.Name == "sherpa4selfie" && !artifact.Expired {
			response = artifact
			err = nil
			break
		}
	}

	return response, err
}

// Source: https://golangcode.com/unzip-files-in-go/
// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
func unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		filePath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", filePath)
		}

		filenames = append(filenames, filePath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func main() {
	var repository string
	var token string
	var directory string

	flag.StringVar(&repository, "r", "pjotrsavitski/sherpa-helper", "Specify GitHub repository. Default is `pjotrsavitski/sherpa-helper`")
	flag.StringVar(&token, "t", "", "Specify authentication token. Default value is an empty string")
	flag.StringVar(&directory, "d", "sherpa4selfie", "Specify asset directory. Default value is `sherpa4selfie` and could also be a fully qualified path")

	flag.Parse()

	if repository == "" || token == "" || directory == "" {
		fmt.Println(colorRed, "At least one of the parameters is missing!", colorReset)
		return
	}

	var updater = updater{repository, token, directory}

	fmt.Println("Downloading artifacts data, please wait ...")
	data, err := updater.Artifacts()

	if err != nil {
		log.Fatal(colorRed, err, colorReset)
	}

	if data.HasArtifacts() {
		artifact, err := data.LatestActiveSherpaArtifact()

		if err != nil {
			log.Fatal(colorRed, err, colorReset)
		}

		err1 := updater.DownloadAndReplace(artifact)

		if err1 != nil {
			log.Fatal(colorRed, err1, colorReset)
		}
	} else {
		fmt.Println(colorBlue, "No artifacts found!", colorReset)
	}

	fmt.Println(colorGreen, "All done", colorReset)
}
