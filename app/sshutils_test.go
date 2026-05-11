package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotateLocalBackups(t *testing.T) {
	dir := t.TempDir()
	// Создаём 5 фейковых папок repoX_date
	for i := 1; i <= 5; i++ {
		path := filepath.Join(dir, fmt.Sprintf("repo1_202301%02d0000", i))
		os.Mkdir(path, 0755)
		// Разное время модификации
		os.Chtimes(path, time.Date(2023, 1, i, 0, 0, 0, 0, time.UTC), time.Date(2023, 1, i, 0, 0, 0, 0, time.UTC))
	}
	err := rotateLocalBackups(dir, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 backups, got %d", count)
	}
}

func TestGenerateAndReadPublicKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test_key")
	err := generateSSHKeyPair(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := readPublicKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) == 0 {
		t.Error("empty public key")
	}
}
