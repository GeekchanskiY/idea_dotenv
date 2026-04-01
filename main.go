package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type Config struct {
	Configurations Configuration `xml:"configuration"`
}

type Configuration struct {
	Envs []Env `xml:"envs>env"`
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s path/to/.runConfigurations\n", os.Args[0])
		os.Exit(1)
	}

	entries, err := os.ReadDir(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error reading directory: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No configuration files found\n")
		os.Exit(0)
	}

	envsCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := strings.Split(entry.Name(), ".")
		if len(name) < 3 {
			_, _ = fmt.Fprintf(os.Stderr, "Invalid configuration file name: %s, skipping\n", entry.Name())
			continue
		}

		filePath := filepath.Join(args[0], entry.Name())

		fileBytes, err := os.ReadFile(filePath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error reading file '%s': %e\n", filePath, err)
			os.Exit(1)
		}

		data := new(Config)
		err = xml.Unmarshal(fileBytes, data)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error parsing file '%s': %e\n", filePath, err)
			os.Exit(1)
		}

		resEnvs := make([]Env, len(data.Configurations.Envs))
		for i, env := range data.Configurations.Envs {
			resEnvs[i] = env
		}

		newFileName := "." + strings.Join(name[:len(name)-2], ".") + ".env"

		_, err = os.Stat(newFileName)
		if !errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintf(os.Stdout, "File '%s' already exists, rewriting...\n", newFileName)
		}

		f, err := os.Create(newFileName)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error creating file '%s': %e\n", newFileName, err)
			os.Exit(1)
		}

		defer func(f *os.File) {
			err := f.Close()
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error closing file '%s': %e\n", newFileName, err)
			}
		}(f)

		for _, env := range resEnvs {
			_, err = f.WriteString(env.Name + "=" + env.Value + "\n")
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error writing file '%s': %e\n", newFileName, err)
				os.Exit(1)
			}
		}

		_, _ = fmt.Fprintf(os.Stdout, "Created file '%s'\n", newFileName)

		envsCount++
	}

	if envsCount == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No configuration files found\n")
		os.Exit(0)
	}

	if envsCount == 1 {
		_, _ = fmt.Fprintf(os.Stdout, "%d environment created successfully\n", envsCount)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "%d environments created successfully\n", envsCount)
	}

}
