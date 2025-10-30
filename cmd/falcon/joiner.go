package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"

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
	//Sort with file name or we will join files with wrong order
	sort.Strings(files)
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
