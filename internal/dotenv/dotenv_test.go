// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package dotenv_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/maloquacious/yaee/internal/dotenv"
)

func TestLoadInvalidEnvironment(t *testing.T) {
	loader := dotenv.New(fstest.MapFS{})
	for _, tc := range []struct {
		env  string
		want error
	}{
		{"", dotenv.ErrMissingEnvironment},
		{"staging", dotenv.ErrUnknownEnvironment},
		{"Development", dotenv.ErrUnknownEnvironment},
	} {
		t.Run(tc.env, func(t *testing.T) {
			if err := loader.Load(tc.env); !errors.Is(err, tc.want) {
				t.Fatalf("Load(%q) = %v, want %v", tc.env, err, tc.want)
			}
		})
	}
}

func TestLoadPrecedence(t *testing.T) {
	for _, env := range []string{"development", "test", "production", "agent"} {
		t.Run(env, func(t *testing.T) {
			for _, key := range []string{"YAEE_TEST_PROCESS", "YAEE_TEST_EMPTY", "YAEE_TEST_ENV_LOCAL", "YAEE_TEST_LOCAL", "YAEE_TEST_ENV", "YAEE_TEST_BASE"} {
				// Register restoration before removing any inherited value.
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("YAEE_TEST_PROCESS", "process")
			t.Setenv("YAEE_TEST_EMPTY", "")

			fsys := fstest.MapFS{}
			for path, data := range map[string]string{
				".env." + env + ".local": "YAEE_TEST_PROCESS=file\nYAEE_TEST_EMPTY=file\nYAEE_TEST_ENV_LOCAL=env-local\n",
				".env.local":             "YAEE_TEST_ENV_LOCAL=local\nYAEE_TEST_LOCAL=local\n",
				".env." + env:            "YAEE_TEST_ENV_LOCAL=env\nYAEE_TEST_LOCAL=env\nYAEE_TEST_ENV=env\n",
				".env":                   "YAEE_TEST_ENV_LOCAL=base\nYAEE_TEST_LOCAL=base\nYAEE_TEST_ENV=base\nYAEE_TEST_BASE=base\n",
			} {
				fsys[path] = &fstest.MapFile{Data: []byte(data)}
			}

			if err := dotenv.New(fsys).Load(env); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{
				"YAEE_TEST_PROCESS":   "process",
				"YAEE_TEST_EMPTY":     "",
				"YAEE_TEST_ENV_LOCAL": "env-local",
				"YAEE_TEST_LOCAL":     "local",
				"YAEE_TEST_ENV":       "env",
				"YAEE_TEST_BASE":      "base",
			} {
				if got, ok := os.LookupEnv(key); !ok || got != want {
					t.Errorf("%s = %q (set=%t), want %q", key, got, ok, want)
				}
			}
		})
	}
}

func TestLoadSkipsMissingAndNonRegularFiles(t *testing.T) {
	fsys := fstest.MapFS{}
	loader := dotenv.New(fsys)
	if err := loader.Load("agent"); err != nil {
		t.Fatalf("Load with no files: %v", err)
	}
	fsys[".env.agent.local"] = &fstest.MapFile{Mode: fs.ModeDir}
	t.Setenv("YAEE_TEST_FALLBACK", "")
	if err := os.Unsetenv("YAEE_TEST_FALLBACK"); err != nil {
		t.Fatal(err)
	}
	fsys[".env"] = &fstest.MapFile{Data: []byte("YAEE_TEST_FALLBACK=base\n")}
	if err := loader.Load("agent"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("YAEE_TEST_FALLBACK"); got != "base" {
		t.Fatalf("fallback = %q, want base", got)
	}
}

func TestLoadStatError(t *testing.T) {
	t.Chdir(t.TempDir())
	const path = ".env.agent.local"
	if err := os.Symlink(path, path); err != nil {
		t.Fatal(err)
	}
	err := dotenv.Load("agent")
	if !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("Load = %v, want wrapped ELOOP", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != path {
		t.Fatalf("Load = %v, want PathError for %s", err, path)
	}
}

func TestLoadParseError(t *testing.T) {
	const path = ".env.agent.local"
	fsys := fstest.MapFS{
		path: &fstest.MapFile{Data: []byte("BROKEN='unterminated\n")},
	}
	if err := dotenv.New(fsys).Load("agent"); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load = %v, want parse error identifying %s", err, path)
	}
}

func TestLoadWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("YAEE_TEST_WORKING_DIR", "")
	if err := os.Unsetenv("YAEE_TEST_WORKING_DIR"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("YAEE_TEST_WORKING_DIR=working-directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dotenv.Load("agent"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("YAEE_TEST_WORKING_DIR"); got != "working-directory" {
		t.Fatalf("value = %q, want working-directory", got)
	}
}
