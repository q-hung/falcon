package main

import (
	"fmt"
	"os"
	"path/filepath"
)

/*
Part struct
*/
type Part struct {
	URL       string
	Path      string
	RangeFrom int64
	RangeTo   int64
}

func calculateParts(connections int64, length int64, url string) []Part {
	parts := make([]Part, 0)
	for i := int64(0); i < connections; i++ {
		fromBytes := (length / connections) * i
		var toBytes int64
		
		if i < connections-1 {
			// For non-last parts: calculate end byte
			toBytes = ((length / connections) * (i + 1)) - 1
		} else {
			// For the last part: end at the last byte (length - 1, since HTTP ranges are 0-indexed)
			toBytes = length - 1
		}

		file := FilenameFromURL(url)
		folder := GetValidFolderPath(url)

		if err := CreateFolderIfNotExist(folder); err != nil {
			HandleError(err)
			os.Exit(1)
		}

		filename := fmt.Sprintf("%s.part%d", file, i)
		path := filepath.Join(folder, filename) // ~/.falcon/download-file-name/part-name

		parts = append(parts, Part{URL: url, Path: path, RangeFrom: fromBytes, RangeTo: toBytes})
	}
	return parts
}
