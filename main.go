package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func DownloadFile(filepath string, url string) error {

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
func Unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
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

type GitHubResponse struct {
	DefaultBranch string `json:"default_branch"`
}

func VerifyBranchName(username string, reponame string, branchName string) bool {
	response, err := http.Get("https://api.github.com/repos/" + username + "/" + reponame + "/branches" + branchName)
	if err != nil {
		check(err)
	}

	if response.StatusCode == http.StatusNotFound {
		return false
	}

	return true
}

func GetMainBranchName(username string, reponame string) (string, error) {
	response, err := http.Get("https://api.github.com/repos/" + username + "/" + reponame)
	if err != nil {
		return "", err
	}

	data, _ := ioutil.ReadAll(response.Body)
	// bodyStr := string(data)
	var obj GitHubResponse
	err = json.Unmarshal(data, &obj)
	if err != nil {
		return "", err
	}

	return obj.DefaultBranch, nil
}

func InitRepo(path string) error {
	gitBin, _ := exec.LookPath("git")

	cmd := &exec.Cmd{
		Path:   gitBin,
		Args:   []string{gitBin, "init", path},
		Stdout: os.Stdout,
		Stdin:  os.Stdin,
	}

	err := cmd.Run()
	return err
}

func CheckIsGitInstalled() bool {
	binPath, _ := exec.LookPath("git")
	if binPath != "" {
		return true
	}
	return false
}

func main() {
	var dstPath string
	var branchName string
	isGitInstalled := CheckIsGitInstalled()
	if isGitInstalled == false {
		fmt.Println("git not found. Repository will not be intialized automatically.")
	}

	branchNamePtr := flag.String("branch", "", "The name of the branch to download.")
	outPtr := flag.String("out", "", "Destination path of the project.")
	shouldNotInitPtr := flag.Bool("no-init", false, "If set, templify will not automatically initialize the repo.")
	flag.Parse()

	tail := flag.Args()

	if len(tail) == 0 {
		fmt.Println("A GitHub url must be speficied.")
		return
	}

	// TODO: Add extra validation to this
	repoUrl := tail[0]
	splitGitUrl := strings.Split(repoUrl, "/")
	username := splitGitUrl[3]
	reponame := splitGitUrl[4]

	// Setup
	if *outPtr == "" {
		dstPath = reponame
	} else {
		dstPath = *outPtr
	}

	// Get main branch
	if *branchNamePtr == "" {
		mainBranchName, err := GetMainBranchName(username, reponame)
		check(err)
		branchName = mainBranchName
	} else {
		existsbranch := VerifyBranchName(username, reponame, *branchNamePtr)
		if !existsbranch {
			fmt.Printf("The branch %v not exists for the repository %v\n", *branchNamePtr, repoUrl)
			os.Exit(1)
		}
		branchName = *branchNamePtr
	}

	// Create temp folder
	tempDir := ".templify-temp"
	err := os.Mkdir(".templify-temp", 0755)
	check(err)

	// Download the archive
	fileUrl := "https://github.com/" + username + "/" + reponame + "/archive/refs/heads/" + branchName + ".zip"
	zipStr := []string{tempDir, "zip.zip"}
	zipFilePath := strings.Join(zipStr, "/")
	DownloadFile(zipFilePath, fileUrl)

	// Unzip the file
	unzipStr := []string{tempDir, "unzipped"}
	unzipPath := strings.Join(unzipStr, "/")
	Unzip(zipFilePath, unzipPath)

	files, err := ioutil.ReadDir(unzipPath)
	check(err)
	repoDir := files[0]

	// Move the folder
	repoDirStr := []string{unzipPath, repoDir.Name()}
	repoDirPath := strings.Join(repoDirStr, "/")
	err = os.Rename(repoDirPath, "./"+dstPath)
	check(err)

	// Remove the temp folder
	defer os.RemoveAll(tempDir)

	// Init the repo
	if isGitInstalled && *shouldNotInitPtr == false {
		InitRepo(dstPath)
	}
}
