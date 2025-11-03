package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// HandleError handle fatal error
func HandleError(err error) {
	if err != nil {
		err := fmt.Errorf("%v", err)
		panic(err)
	}
}

// GetUserHome returns default user home folder
func GetUserHome() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	return usr.HomeDir
}

// IsValidURL Check if given string is valid url
func IsValidURL(s string) bool {
	_, err := neturl.ParseRequestURI(s)
	return err == nil
}

// CreateFolderIfNotExist Create new directory if it doesn't exist
func CreateFolderIfNotExist(folder string) error {
	if _, err := os.Stat(folder); err != nil {
		if err = os.MkdirAll(folder, 0755); err != nil {
			return err
		}
	}
	return nil
}

// FilenameFromURL generate safe filename from url
func FilenameFromURL(rawURL string) string {
	url, err := neturl.Parse(rawURL)

	HandleError(err)

	slugs := strings.Split(url.EscapedPath(), "/")

	if len(slugs) == 1 {
		return slugs[0]
	}

	slug := slugs[len(slugs)-1]

	if slug == "" && len(slugs) > 1 {
		slug = slugs[len(slugs)-2]
	}

	return slug
}

func makeMd5(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

// func base64encode(text string) string {
// 	return b64.StdEncoding.EncodeToString([]byte(text))
// }

func tempFolderName(text string) string {
	return makeMd5(text)
}

// GetValidFolderPath must ensure full qualify path is CHILD of safe path
func GetValidFolderPath(url string) string {
	safePath := filepath.Join(GetUserHome(), appHome)
	fullQualifyPath, err := filepath.Abs(filepath.Join(GetUserHome(), appHome, tempFolderName(url)))
	HandleError(err)

	//must ensure full qualify path is CHILD of safe path
	//to prevent directory traversal attack
	//using Rel function to get relative between parent and child
	//if relative join base == child, then child path MUST BE real child
	relative, err := filepath.Rel(safePath, fullQualifyPath)
	HandleError(err)

	if strings.Contains(relative, "..") {
		HandleError(errors.New("you may be a victim of directory traversal path attack"))
		return ""
	}

	return fullQualifyPath
}

// FilterIPV4 Filter out list of ipv4
func FilterIPV4(ips []net.IP) []string {
	var ret = make([]string, 0)
	for _, ip := range ips {
		if ip.To4() != nil {
			ret = append(ret, ip.String())
		}
	}
	return ret
}
