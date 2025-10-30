package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

func main() {
	var conn int

	var cmdGet = &cobra.Command{
		Use:   "get [url]",
		Short: "Download the given url",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			get(args[0], conn, nil)
		},
	}

	cmdGet.Flags().IntVarP(&conn, "connection", "c", runtime.NumCPU(), "The number of connections")

	//@TODO Bug when resume 3/4 parts
	var cmdResume = &cobra.Command{
		Use:   "resume [task]",
		Short: "Resume an unfinished task",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resume(args[0], conn)
		},
	}

	var cmdTask = &cobra.Command{
		Use:   "tasks",
		Short: "Listing all unfinished tasks",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			listTasks()
		},
	}

	var rootCmd = &cobra.Command{Use: "falcon"}
	rootCmd.AddCommand(cmdGet, cmdResume, cmdTask)
	rootCmd.Execute()
}

func get(url string, conn int, state *State) {
	var downloader *HttpDownloader
	if state == nil {
		downloader = NewHttpDownloader(url, int64(conn), make([]Part, 0))
	} else {
		downloader = NewHttpDownloader(url, int64(conn), state.Parts)
	}

	downloader.Start()
}

func resume(task string, conn int) {
	state, err := LoadState(task)
	HandleError(err)
	get(state.URL, conn, state)
}

func listTasks() {
	files, err := os.ReadDir(filepath.Join(GetUserHome(), appHome))

	if err != nil {
		log.Fatal(err)
	}

	if len(files) == 0 {
		fmt.Println("There is no unfinished task")
	}

	for _, f := range files {
		fmt.Println(f.Name())
	}
}
