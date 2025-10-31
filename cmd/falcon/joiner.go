package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/fatih/color"
)

func isJoinable(files []string) bool {
	if len(files) == 1 {
		fi, e := os.Stat(files[0])
		HandleError(e)
		if fi.Size() == 0 {
			return false
		}
	}
	return true
}

// JoinFile combine a list of files into single file
func JoinFile(files []string, out string) error {
	// Sort files by part number (numeric order, not string order)
	// This handles cases like part0, part1, part10, part2 correctly
	sort.Slice(files, func(i, j int) bool {
		// Extract part number from filename (e.g., "file.part10" -> 10)
		baseI := filepath.Base(files[i])
		baseJ := filepath.Base(files[j])

		// Find the part number
		partsI := strings.Split(baseI, ".part")
		partsJ := strings.Split(baseJ, ".part")

		if len(partsI) < 2 || len(partsJ) < 2 {
			// Fallback to string comparison if format is unexpected
			return files[i] < files[j]
		}

		numI, errI := strconv.Atoi(partsI[len(partsI)-1])
		numJ, errJ := strconv.Atoi(partsJ[len(partsJ)-1])

		if errI != nil || errJ != nil {
			// Fallback to string comparison if parsing fails
			return files[i] < files[j]
		}

		return numI < numJ
	})
	fmt.Println("Start joining")
	var bar *pb.ProgressBar
	prefix := "Joining"

	if runtime.GOOS != "windows" {
		prefix = color.GreenString(prefix)
	}

	bar = pb.StartNew(len(files)).Set("prefix", prefix)

	outf, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer outf.Close()

	for _, f := range files {
		if err = copy(f, outf); err != nil {
			return err
		}
		bar.Increment()
	}

	bar.Finish()

	return nil
}

func copy(from string, to io.Writer) error {
	f, err := os.OpenFile(from, os.O_RDONLY, 0600)
	HandleError(err)
	defer f.Close()
	if err != nil {
		return err
	}
	io.Copy(to, f)
	return nil
}
