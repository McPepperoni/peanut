package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/config"
)

func TestRunMainInitializesDatabaseAndDispatchesArguments(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "peanut.db")
	ran := false
	err := runMain(context.Background(), []string{"peanut", "run"}, databasePath, commandDependencies{
		Run: func(context.Context) error {
			ran = true
			return nil
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("run command was not dispatched")
	}
	if _, err := config.Load(databasePath); err != nil {
		t.Fatalf("load initialized database: %v", err)
	}
}

func TestRunMainWithoutArgumentsReturnsUsage(t *testing.T) {
	err := runMain(context.Background(), nil, filepath.Join(t.TempDir(), "peanut.db"), commandDependencies{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error = %v, want usage", err)
	}
}
