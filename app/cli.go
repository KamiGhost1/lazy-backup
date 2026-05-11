package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func runCLI() {
	repoID := flag.Int("repo", 0, "ID репозитория")
	action := flag.String("action", "", "download или upload")
	showPub := flag.Int("show-pubkey", 0, "Показать публичный ключ сервера по ID")
	listServers := flag.Bool("list-servers", false, "Список серверов")
	listRepos := flag.Bool("list-repos", false, "Список репозиториев (нужен -server)")
	serverID := flag.Int("server", 0, "ID сервера для списка репозиториев")
	flag.Parse()

	db, err := initDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if *showPub > 0 {
		s, err := getServerByID(db, *showPub)
		if err != nil {
			log.Fatalf("Сервер не найден: %v", err)
		}
		if s.KeyPath == "" {
			log.Fatal("У сервера не указан путь к ключу")
		}
		pub, err := readPublicKey(s.KeyPath)
		if err != nil {
			log.Fatalf("Ошибка чтения публичного ключа: %v", err)
		}
		fmt.Printf("Публичный ключ сервера %s:\n%s", s.Name, pub)
		return
	}

	if *listServers {
		servers, err := loadServers(db)
		if err != nil {
			log.Fatal(err)
		}
		for _, s := range servers {
			fmt.Printf("[%d] %s\n", s.ID, formatServerItem(s))
		}
		return
	}

	if *listRepos {
		if *serverID == 0 {
			log.Fatal("Укажите -server ID")
		}
		repos, err := loadReposForServer(db, *serverID)
		if err != nil {
			log.Fatal(err)
		}
		for _, r := range repos {
			fmt.Printf("[%d] %s\n", r.ID, formatRepoItem(r))
		}
		return
	}

	if *repoID == 0 || *action == "" {
		fmt.Println("Использование: backup-manager -repo=<id> -action=<download|upload>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	repo, err := getRepoByID(db, *repoID)
	if err != nil {
		log.Fatalf("Репозиторий не найден: %v", err)
	}
	server, err := getServerByID(db, repo.ServerID)
	if err != nil {
		log.Fatalf("Сервер не найден: %v", err)
	}

	localDir := "backups"
	var status, details string

	switch *action {
	case "download":
		msg, err := downloadBackup(server, repo, localDir)
		if err != nil {
			status = "failed"
			details = err.Error()
		} else {
			status = "completed"
			details = msg
		}
		fmt.Println(details)
		_ = insertBackupLog(db, repo.ID, "download", status, details)
	case "upload":
		msg, err := uploadBackup(server, repo, localDir)
		if err != nil {
			status = "failed"
			details = err.Error()
		} else {
			status = "completed"
			details = msg
		}
		fmt.Println(details)
		_ = insertBackupLog(db, repo.ID, "upload", status, details)
	default:
		log.Fatalf("Неизвестное действие: %s", *action)
	}
}
