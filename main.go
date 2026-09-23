package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type Config struct {
	Configurations []Configuration `xml:"configuration"`
}

type Configuration struct {
	Name string `xml:"name,attr"`
	Envs []Env  `xml:"envs>env"`
}

var (
	errUsage = errors.New("usage")
	errHelp  = errors.New("help")
)

func main() {
	inDir, outDir, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
			fmt.Fprint(os.Stdout, usageText())
			os.Exit(0)
		}
		fmt.Fprint(os.Stderr, usageText())
		os.Exit(1)
	}

	n, err := convert(inDir, outDir, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if n == 0 {
		fmt.Fprintln(os.Stdout, "No configuration files found")
		return
	}
	if n == 1 {
		fmt.Fprintf(os.Stdout, "%d environment created successfully\n", n)
		return
	}
	fmt.Fprintf(os.Stdout, "%d environments created successfully\n", n)
}

func usageText() string {
	return fmt.Sprintf("Usage: %s [-o dir] path/to/.runConfigurations\n", filepath.Base(os.Args[0]))
}

func parseArgs(args []string) (string, string, error) {
	fs := flag.NewFlagSet("idea_dotenv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("o", "", "directory to write .env files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", "", errHelp
		}
		return "", "", errUsage
	}
	if fs.NArg() != 1 {
		return "", "", errUsage
	}

	inDir := fs.Arg(0)
	outDir := *out
	if outDir == "" {
		outDir = filepath.Dir(inDir)
	}
	return inDir, outDir, nil
}

func convert(inDir, outDir string, log io.Writer) (int, error) {
	entries, err := os.ReadDir(inDir)
	if err != nil {
		return 0, fmt.Errorf("reading directory: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating output directory '%s': %w", outDir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".run.xml") {
			continue
		}
		n, err := convertFile(filepath.Join(inDir, entry.Name()), outDir, log)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

func convertFile(path, outDir string, log io.Writer) (int, error) {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("reading file '%s': %w", path, err)
	}

	var data Config
	if err := xml.Unmarshal(fileBytes, &data); err != nil {
		return 0, fmt.Errorf("parsing file '%s': %w", path, err)
	}
	if len(data.Configurations) == 0 {
		return 0, fmt.Errorf("parsing file '%s': no run configurations", path)
	}

	names, err := outputNames(filepath.Base(path), data.Configurations)
	if err != nil {
		return 0, err
	}
	for _, cfg := range data.Configurations {
		if len(cfg.Envs) == 0 {
			continue
		}
		if err := validateEnvs(cfg.Envs); err != nil {
			return 0, fmt.Errorf("file '%s': %w", filepath.Base(path), err)
		}
	}

	written := 0
	for i, cfg := range data.Configurations {
		if len(cfg.Envs) == 0 {
			continue
		}

		dest := filepath.Join(outDir, names[i])
		if err := prepareDest(dest, log); err != nil {
			return written, err
		}
		if err := writeEnvFile(dest, cfg.Envs); err != nil {
			return written, err
		}
		fmt.Fprintf(log, "Created file '%s'\n", dest)
		written++
	}
	if written == 0 {
		fmt.Fprintf(log, "Skipping '%s': no environment variables\n", filepath.Base(path))
	}
	return written, nil
}

func outputNames(runFile string, configs []Configuration) ([]string, error) {
	names := make([]string, len(configs))
	if len(configs) == 1 {
		if len(configs[0].Envs) == 0 {
			return names, nil
		}
		base := strings.TrimSuffix(runFile, ".run.xml")
		if !safeBase(base) {
			return nil, fmt.Errorf("invalid configuration file name: %s", runFile)
		}
		names[0] = dotenvFileName(base)
		return names, nil
	}

	seen := make(map[string]struct{}, len(configs))
	for i, cfg := range configs {
		if len(cfg.Envs) == 0 {
			continue
		}
		name := cfg.Name
		if !safeBase(name) {
			return nil, fmt.Errorf("configuration name %q in '%s' is not a safe file name", name, runFile)
		}
		file := dotenvFileName(name)
		if _, ok := seen[file]; ok {
			return nil, fmt.Errorf("duplicate configuration name %q in '%s'", name, runFile)
		}
		seen[file] = struct{}{}
		names[i] = file
	}
	return names, nil
}

func safeBase(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return name == filepath.Base(name) && !strings.ContainsAny(name, `/\`)
}

func dotenvFileName(base string) string {
	if strings.HasPrefix(base, ".") {
		return base + ".env"
	}
	return "." + base + ".env"
}

func prepareDest(path string, log io.Writer) error {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking file '%s': %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("checking file '%s': is a directory", path)
	}
	fmt.Fprintf(log, "File '%s' already exists, rewriting...\n", path)
	return nil
}

func writeEnvFile(path string, envs []Env) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".idea_dotenv-*")
	if err != nil {
		return fmt.Errorf("creating temp file for '%s': %w", path, err)
	}
	tmp := f.Name()

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("setting permissions on '%s': %w", path, err)
	}

	for _, env := range envs {
		line, err := formatEnvLine(env.Name, env.Value)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := f.WriteString(line); err != nil {
			_ = f.Close()
			return fmt.Errorf("writing file '%s': %w", path, err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing file '%s': %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing file '%s': %w", path, err)
	}
	cleanup = false
	return nil
}

func validateEnvs(envs []Env) error {
	for _, env := range envs {
		if _, err := formatEnvLine(env.Name, env.Value); err != nil {
			return err
		}
	}
	return nil
}

func formatEnvLine(name, value string) (string, error) {
	if !validEnvName(name) {
		return "", fmt.Errorf("invalid environment variable name %q", name)
	}
	if !needsQuotes(value) {
		return name + "=" + value + "\n", nil
	}
	return name + "=" + quoteEnvValue(value) + "\n", nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == '=' || r == '#' || r == '"' || r == '\'' || r == '\\' || r == 0x7f {
			return false
		}
	}
	return true
}

func needsQuotes(value string) bool {
	if value == "" {
		return false
	}
	if strings.TrimSpace(value) != value {
		return true
	}
	return strings.ContainsAny(value, " \t\"'#\\\n\r$")
}

func quoteEnvValue(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(value[i])
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(value[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}
