package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExampleFixture(t *testing.T) {
	out := t.TempDir()
	var log strings.Builder
	n, err := convert("examples/default/.runConfigurations", out, &log)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("converted %d files, want 1", n)
	}

	got, err := os.ReadFile(filepath.Join(out, ".example.env"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("examples/default/.example.env")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("env file =\n%q\nwant\n%q", got, want)
	}
	assertMode(t, filepath.Join(out, ".example.env"))
}

func TestDotenvNameKeepsExistingLeadingDot(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, filepath.Join(in, ".local.run.xml"), `
		<component name="ProjectRunConfigurationManager">
		  <configuration name="ignored" type="GoApplicationRunConfiguration">
		    <envs><env name="TOKEN" value="abc" /></envs>
		  </configuration>
		</component>`)

	out := t.TempDir()
	if _, err := convert(in, out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, ".local.env")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "..local.env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected ..local.env: %v", err)
	}
}

func TestSkipsUnrelatedFilesAndEmptyEnvs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "notes.txt.bak"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, filepath.Join(in, "empty.run.xml"), `
		<component name="ProjectRunConfigurationManager">
		  <configuration name="empty" type="GoApplicationRunConfiguration">
		    <envs></envs>
		  </configuration>
		</component>`)

	out := t.TempDir()
	var log strings.Builder
	n, err := convert(in, out, &log)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("converted %d files, want 0", n)
	}
	if !strings.Contains(log.String(), "Skipping 'empty.run.xml'") {
		t.Fatalf("log = %q", log.String())
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("output dir has %d entries, want 0", len(entries))
	}
}

func TestMultipleConfigurations(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, filepath.Join(in, "shared.run.xml"), `
		<component name="ProjectRunConfigurationManager">
		  <configuration name="one" type="GoApplicationRunConfiguration">
		    <envs><env name="A" value="1" /></envs>
		  </configuration>
		  <configuration name="two" type="GoApplicationRunConfiguration">
		    <envs><env name="B" value="hello world" /></envs>
		  </configuration>
		</component>`)

	out := t.TempDir()
	n, err := convert(in, out, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("converted %d files, want 2", n)
	}
	assertFile(t, filepath.Join(out, ".one.env"), "A=1\n")
	assertFile(t, filepath.Join(out, ".two.env"), "B=\"hello world\"\n")
}

func TestQuotesSpecialValues(t *testing.T) {
	line, err := formatEnvLine("SAY", "say \"hi\"")
	if err != nil {
		t.Fatal(err)
	}
	if line != "SAY=\"say \\\"hi\\\"\"\n" {
		t.Fatalf("quoted line = %q", line)
	}

	line, err = formatEnvLine("NOTE", "a#b")
	if err != nil {
		t.Fatal(err)
	}
	if line != "NOTE=\"a#b\"\n" {
		t.Fatalf("hash line = %q", line)
	}

	line, err = formatEnvLine("TEXT", "line1\nline2")
	if err != nil {
		t.Fatal(err)
	}
	if line != "TEXT=\"line1\\nline2\"\n" {
		t.Fatalf("newline line = %q", line)
	}

	line, err = formatEnvLine("PATH", "$PROJECT_DIR$/bin")
	if err != nil {
		t.Fatal(err)
	}
	if line != "PATH=\"$PROJECT_DIR$/bin\"\n" {
		t.Fatalf("macro line = %q", line)
	}
}

func TestInvalidXMLDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "broken.run.xml"), []byte("<component>"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	dest := filepath.Join(out, ".broken.env")
	if err := os.WriteFile(dest, []byte("KEEP=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := convert(in, out, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "parsing file") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "%!e") {
		t.Fatalf("error is misformatted: %v", err)
	}
	assertFile(t, dest, "KEEP=1\n")
}

func TestInvalidNameDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, filepath.Join(in, "bad.run.xml"), `
		<component name="ProjectRunConfigurationManager">
		  <configuration name="bad" type="GoApplicationRunConfiguration">
		    <envs><env name="BAD NAME" value="x" /></envs>
		  </configuration>
		</component>`)

	out := t.TempDir()
	dest := filepath.Join(out, ".bad.env")
	if err := os.WriteFile(dest, []byte("KEEP=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	_, err := convert(in, out, &log)
	if err == nil {
		t.Fatal("expected invalid name error")
	}
	if strings.Contains(log.String(), "rewriting") {
		t.Fatalf("rewrote before validation: %s", log.String())
	}
	assertFile(t, dest, "KEEP=1\n")
}

func TestInvalidLaterConfigWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "runs")
	if err := os.Mkdir(in, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, filepath.Join(in, "shared.run.xml"), `
		<component name="ProjectRunConfigurationManager">
		  <configuration name="one" type="GoApplicationRunConfiguration">
		    <envs><env name="A" value="1" /></envs>
		  </configuration>
		  <configuration name="two" type="GoApplicationRunConfiguration">
		    <envs><env name="BAD NAME" value="x" /></envs>
		  </configuration>
		</component>`)

	out := t.TempDir()
	if _, err := convert(in, out, io.Discard); err == nil {
		t.Fatal("expected invalid name error")
	}
	if _, err := os.Stat(filepath.Join(out, ".one.env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial env file: %v", err)
	}
}

func TestParseArgs(t *testing.T) {
	in, out, err := parseArgs([]string{"examples/default/.runConfigurations"})
	if err != nil {
		t.Fatal(err)
	}
	if in != "examples/default/.runConfigurations" || out != filepath.Dir(in) {
		t.Fatalf("in=%q out=%q", in, out)
	}

	in, out, err = parseArgs([]string{"-o", "custom", "runs"})
	if err != nil {
		t.Fatal(err)
	}
	if in != "runs" || out != "custom" {
		t.Fatalf("in=%q out=%q", in, out)
	}

	if _, _, err := parseArgs(nil); !errors.Is(err, errUsage) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := parseArgs([]string{"-h"}); !errors.Is(err, errHelp) {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingDirectoryReportsUnderlyingError(t *testing.T) {
	_, err := convert(filepath.Join(t.TempDir(), "missing"), t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "%!e") || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
}

func writeRunFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s =\n%q\nwant\n%q", path, got, want)
	}
	assertMode(t, path)
}

func assertMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode = %o, want 600", path, fi.Mode().Perm())
	}
}
