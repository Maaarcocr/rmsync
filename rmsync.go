package rmsync

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var myClient = &http.Client{Timeout: 5 * time.Minute}

const baseDir string = "/home/root/.local/share/remarkable/xochitl/"

type metadata struct {
	Deleted          bool   `json:"deleted"`
	DastModified     string `json:"lastModified"`
	Metadatamodified bool   `json:"metadatamodified"`
	Modified         bool   `json:"modified"`
	Parent           string `json:"parent"`
	Pinned           bool   `json:"pinned"`
	Synced           bool   `json:"synced"`
	Type             string `json:"type"`
	Version          int    `json:"version"`
	VisibleName      string `json:"visibleName"`
}

// A file in the reMarkable tablet has a filename, in the form of: "<some-uuid>.<some-extension>"
// and a visible name, which is what users see in their screens.
type RemarkableFile struct {
	Filename    string
	VisibleName string
}

// A file that needs to be synced by this library, it must have a name and an URL that will be used to download the file.
// The name should be unique, as it will be used to decide if to download or not a file when the `Sync` function
// iterates over the list of FileToSync given as an argument to it.
type FileToSync struct {
	Filename string
	Url      string
}

func getMetadataFilenames() ([]string, error) {
	var filenames []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".metadata" {
			return nil
		}
		filenames = append(filenames, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return filenames, nil
}

// In case you want to do something with the directories existing in the reMarkable,
// you can use this to get their filenames and visible names.
func GetDirectoriesMetadataFiles() ([]RemarkableFile, error) {
	filenames, err := getMetadataFilenames()
	if err != nil {
		return nil, err
	}
	var directories []RemarkableFile
	for _, filename := range filenames {
		filecontent, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		var metadata metadata
		err = json.Unmarshal(filecontent, &metadata)
		if err != nil {
			return nil, err
		}
		if metadata.Type == "CollectionType" {
			directories = append(directories, RemarkableFile{filename, metadata.VisibleName})
		}
	}
	return directories, nil
}

// In case you want to do something with the pdf files existing in the reMarkable,
// you can use this to get their filenames and visible names.
func GetPdfFiles() ([]RemarkableFile, error) {
	filenames, err := getMetadataFilenames()
	if err != nil {
		return nil, err
	}
	var pdfFiles []RemarkableFile
	for _, filename := range filenames {
		filecontent, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		var metadata metadata
		err = json.Unmarshal(filecontent, &metadata)
		if err != nil {
			return nil, err
		}
		if metadata.Type == "DocumentType" {
			pdfFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".pdf"
			if _, err := os.Stat(pdfFilename); !os.IsNotExist(err) {
				pdfFiles = append(pdfFiles, RemarkableFile{pdfFilename, metadata.VisibleName})
			}
		}
	}
	return pdfFiles, nil
}

func createRemarkableFileMap(files []RemarkableFile) map[string]struct{} {
	fileMap := make(map[string]struct{}, 0)
	for _, file := range files {
		fileMap[file.VisibleName] = struct{}{}
	}
	return fileMap
}

func downloadPdfFile(url string) ([]byte, error) {
	r, err := myClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	fileContents, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return fileContents, nil
}

// This functions uploads a pdf, here represented as a `[]byte` to the reMarkable. This is a low level utility,
// ideally you should use the `Sync` function and give it a list of pdfs that you want to download.
func UploadPdfToTablet(fileContents []byte, filename string) error {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	part.Write(fileContents)

	err = writer.Close()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", "http://10.11.99.1/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	if err != nil {
		return err
	}
	_, err = myClient.Do(req)
	if err != nil {
		return err
	}
	return nil
}

// This function takes a list of pdf files that you want to sync. You must provide both their filenames and a public
// URL that the tablet can use to download them. When iterating through the list of files to sync, this function
// will first check if a file with the same name already exists, if so it will not download it again, if not it will download it
// and then add it to the remarkable files.
func Sync(files []FileToSync) error {
	pdfFiles, err := GetPdfFiles()
	if err != nil {
		return err
	}
	pdfFileMap := createRemarkableFileMap(pdfFiles)

	for _, item := range files {
		if _, ok := pdfFileMap[item.Filename]; !ok {
			fileContents, err := downloadPdfFile(item.Url)
			if err != nil {
				return err
			}
			err = UploadPdfToTablet(fileContents, item.Filename)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
