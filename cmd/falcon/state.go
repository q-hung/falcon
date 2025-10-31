package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State struct to store current state
type State struct {
	URL   string
	Parts []Part
}

// Save current state to .json file
func (s *State) Save() error {
	folder := GetValidFolderPath(s.URL)
	fmt.Printf("Saving current download data in %s\n", folder)
	if err := CreateFolderIfNotExist(folder); err != nil {
		return err
	}

	//move current downloading file to data folder
	for _, part := range s.Parts {
		os.Rename(part.Path, filepath.Join(folder, filepath.Base(part.Path)))
	}

	//save state file
	j, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(folder, stateFileName), j, 0644)
}

// LoadState state.json from unfinished stask folder
func LoadState(task string) (*State, error) {
	file := filepath.Join(GetUserHome(), appHome, task, stateFileName)
	fmt.Printf("Getting data from %s\n", file)
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	s := new(State)
	err = json.Unmarshal(bytes, s)
	return s, err
}
