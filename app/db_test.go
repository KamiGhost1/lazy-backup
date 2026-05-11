package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 22,
		user TEXT NOT NULL,
		password TEXT,
		key_path TEXT
	);
	CREATE TABLE IF NOT EXISTS backup_repos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id INTEGER,
		path TEXT NOT NULL,
		name TEXT NOT NULL,
		max_backups INTEGER DEFAULT 5,
		FOREIGN KEY (server_id) REFERENCES servers(id)
	);
	CREATE TABLE IF NOT EXISTS backup_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id INTEGER,
		direction TEXT NOT NULL,
		status TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		details TEXT,
		FOREIGN KEY (repo_id) REFERENCES backup_repos(id)
	);
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAddAndGetServer(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	err := addServerDB(db, "srv1", "host1", "22", "user1", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := getServerByName(db, "srv1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "srv1" || s.Host != "host1" {
		t.Errorf("unexpected server data: %+v", s)
	}
}

func TestAddAndGetRepo(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	addServerDB(db, "srv1", "host1", "22", "user1", "", "")
	servers, _ := loadServers(db)
	if len(servers) == 0 {
		t.Fatal("server not added")
	}
	serverID := servers[0].ID
	err := addRepoDB(db, "repo1", "/path", serverID, 3)
	if err != nil {
		t.Fatal(err)
	}
	repos, err := loadReposForServer(db, serverID)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].MaxBackups != 3 {
		t.Errorf("expected max_backups=3, got %d", repos[0].MaxBackups)
	}
}

func TestFormatFunctions(t *testing.T) {
	s := Server{Name: "srv", Host: "h", User: "u"}
	if formatServerItem(s) != "srv (u@h)" {
		t.Error("bad formatServerItem")
	}
	r := BackupRepo{Name: "repo", Path: "/p", MaxBackups: 5}
	if formatRepoItem(r) != "repo: /p (max 5)" {
		t.Error("bad formatRepoItem")
	}
}
