package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure-Go драйвер
)

func initDB() (*sql.DB, error) {
	// используем имя драйвера "sqlite"
	db, err := sql.Open("sqlite", "backup_manager.db")
	if err != nil {
		return nil, err
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
	return db, err
}

func addServerDB(db *sql.DB, name, host, port, user, password, keyPath string) error {
	_, err := db.Exec("INSERT INTO servers (name, host, port, user, password, key_path) VALUES (?, ?, ?, ?, ?, ?)",
		name, host, port, user, password, keyPath)
	return err
}

func loadServers(db *sql.DB) ([]Server, error) {
	rows, err := db.Query("SELECT id, name, host, port, user, password, key_path FROM servers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.Password, &s.KeyPath); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	return servers, nil
}

func deleteServerDB(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM servers WHERE id = ?", id)
	return err
}

func getServerByName(db *sql.DB, name string) (Server, error) {
	var s Server
	err := db.QueryRow("SELECT id, name, host, port, user, password, key_path FROM servers WHERE name = ?",
		name).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.Password, &s.KeyPath)
	return s, err
}

func getServerByID(db *sql.DB, id int) (Server, error) {
	var s Server
	err := db.QueryRow("SELECT id, name, host, port, user, password, key_path FROM servers WHERE id = ?",
		id).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.Password, &s.KeyPath)
	return s, err
}

func addRepoDB(db *sql.DB, name, path string, serverID, maxBackups int) error {
	_, err := db.Exec("INSERT INTO backup_repos (name, path, server_id, max_backups) VALUES (?, ?, ?, ?)",
		name, path, serverID, maxBackups)
	return err
}

func loadReposForServer(db *sql.DB, serverID int) ([]BackupRepo, error) {
	rows, err := db.Query("SELECT id, name, path, max_backups FROM backup_repos WHERE server_id = ?", serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []BackupRepo
	for rows.Next() {
		var r BackupRepo
		if err := rows.Scan(&r.ID, &r.Name, &r.Path, &r.MaxBackups); err != nil {
			continue
		}
		r.ServerID = serverID
		repos = append(repos, r)
	}
	return repos, nil
}

func deleteRepoDB(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM backup_repos WHERE id = ?", id)
	return err
}

func getRepoByID(db *sql.DB, repoID int) (BackupRepo, error) {
	var r BackupRepo
	err := db.QueryRow("SELECT id, server_id, name, path, max_backups FROM backup_repos WHERE id = ?",
		repoID).Scan(&r.ID, &r.ServerID, &r.Name, &r.Path, &r.MaxBackups)
	return r, err
}

func getRepoIDByNameAndServer(db *sql.DB, name string, serverID int) (int, error) {
	var id int
	err := db.QueryRow("SELECT id FROM backup_repos WHERE name = ? AND server_id = ?", name, serverID).Scan(&id)
	return id, err
}

func insertBackupLog(db *sql.DB, repoID int, direction, status, details string) error {
	_, err := db.Exec("INSERT INTO backup_logs (repo_id, direction, status, details) VALUES (?, ?, ?, ?)",
		repoID, direction, status, details)
	return err
}

func serverNameByID(db *sql.DB, id int) string {
	var name string
	db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	return name
}

func formatServerItem(s Server) string {
	return fmt.Sprintf("%s (%s@%s)", s.Name, s.User, s.Host)
}

func formatRepoItem(r BackupRepo) string {
	return fmt.Sprintf("%s: %s (max %d)", r.Name, r.Path, r.MaxBackups)
}

func extractRepoName(item string) string {
	parts := strings.SplitN(item, ":", 2)
	return strings.TrimSpace(parts[0])
}

// extractServerName extracts raw name from formatted "name (user@host)" list item.
func extractServerName(item string) string {
	if idx := strings.Index(item, " ("); idx >= 0 {
		return strings.TrimSpace(item[:idx])
	}
	return strings.TrimSpace(item)
}
