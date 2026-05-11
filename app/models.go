package main

import "time"

type Server struct {
	ID       int
	Name     string
	Host     string
	Port     int
	User     string
	Password string
	KeyPath  string
}

type BackupRepo struct {
	ID         int
	ServerID   int
	Path       string
	Name       string
	MaxBackups int
}

type BackupLog struct {
	ID        int
	RepoID    int
	Direction string
	Status    string
	Timestamp time.Time
	Details   string
}
