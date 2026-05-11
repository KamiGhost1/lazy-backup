package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func generateSSHKeyPair(keyPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	privFile, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer privFile.Close()

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privFile, privBlock); err != nil {
		return err
	}
	os.Chmod(keyPath, 0600)

	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}
	pubBytes := ssh.MarshalAuthorizedKey(pub)
	return os.WriteFile(keyPath+".pub", pubBytes, 0644)
}

func readPublicKey(keyPath string) (string, error) {
	data, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// rsyncAvailable проверяет, есть ли rsync в системе
func rsyncAvailable() bool {
	_, err := exec.LookPath("rsync")
	return err == nil
}

func downloadBackup(s Server, r BackupRepo, localBackupDir string) (string, error) {
	if err := os.MkdirAll(localBackupDir, 0755); err != nil {
		return "", err
	}
	timestamp := time.Now().Format("20060102_150405")
	destName := fmt.Sprintf("repo%d_%s", r.ID, timestamp)
	localPath := filepath.Join(localBackupDir, destName)

	var err error
	if rsyncAvailable() {
		err = copyUsingRsync(s, r.Path, localPath, "download")
	} else {
		err = copyUsingSFTP(s, r.Path, localPath, "download")
	}
	if err != nil {
		return "", err
	}

	if r.MaxBackups > 0 {
		rotateLocalBackups(localBackupDir, r.ID, r.MaxBackups)
	}
	return fmt.Sprintf("успешно скачано в %s", localPath), nil
}

func uploadBackup(s Server, r BackupRepo, localBackupDir string) (string, error) {
	prefix := fmt.Sprintf("repo%d_", r.ID)
	entries, err := os.ReadDir(localBackupDir)
	if err != nil {
		return "", fmt.Errorf("нет локальных бекапов: %v", err)
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latest = name
			}
		}
	}
	if latest == "" {
		return "", fmt.Errorf("не найден локальный бекап для repo ID %d", r.ID)
	}
	localPath := filepath.Join(localBackupDir, latest)

	var err2 error
	if rsyncAvailable() {
		err2 = copyUsingRsync(s, localPath, r.Path, "upload")
	} else {
		err2 = copyUsingSFTP(s, localPath, r.Path, "upload")
	}
	if err2 != nil {
		return "", err2
	}
	if r.MaxBackups > 0 {
		go rotateRemoteBackups(s, r)
	}
	return fmt.Sprintf("успешно загружено на %s:%s", s.Name, r.Path), nil
}

// connectSSH устанавливает SSH-соединение
func connectSSH(s Server) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            s.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if s.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(s.Password))
	}
	if s.KeyPath != "" {
		key, err := os.ReadFile(s.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("unable to parse private key: %v", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", s.Host, s.Port), config)
}

// copyUsingRsync использует rsync
func copyUsingRsync(s Server, src, dst, direction string) error {
	var args []string
	if direction == "download" {
		args = []string{"-avz", fmt.Sprintf("%s@%s:%s", s.User, s.Host, src), dst}
	} else {
		args = []string{"-avz", src, fmt.Sprintf("%s@%s:%s", s.User, s.Host, dst)}
	}
	cmd := exec.Command("rsync", args...)
	if s.Password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("RSYNC_PASSWORD=%s", s.Password))
	}
	if s.KeyPath != "" {
		rsh := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no", s.KeyPath)
		cmd.Env = append(cmd.Env, "RSYNC_RSH="+rsh)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync error: %v, output: %s", err, string(out))
	}
	return nil
}

// copyUsingSFTP использует SFTP
func copyUsingSFTP(s Server, src, dst, direction string) error {
	client, err := connectSSH(s)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %v", err)
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("SFTP init failed: %v", err)
	}
	defer sftpClient.Close()

	if direction == "download" {
		// src - удалённый путь, dst - локальный
		return sftpDownload(sftpClient, src, dst)
	} else {
		// src - локальный, dst - удалённый
		return sftpUpload(sftpClient, src, dst)
	}
}

func sftpDownload(client *sftp.Client, remotePath, localPath string) error {
	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	return err
}

func sftpUpload(client *sftp.Client, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	return err
}

func rotateLocalBackups(localDir string, repoID, maxBackups int) error {
	if maxBackups <= 0 {
		return nil
	}
	prefix := fmt.Sprintf("repo%d_", repoID)
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}
	var backups []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			backups = append(backups, e)
		}
	}
	if len(backups) <= maxBackups {
		return nil
	}
	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := backups[i].Info()
		infoJ, _ := backups[j].Info()
		return infoI.ModTime().Before(infoJ.ModTime())
	})
	toDelete := len(backups) - maxBackups
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(localDir, backups[i].Name())
		os.RemoveAll(path)
	}
	return nil
}

func rotateRemoteBackups(s Server, r BackupRepo) {
	if r.MaxBackups <= 0 {
		return
	}
	client, err := connectSSH(s)
	if err != nil {
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()
	cmd := fmt.Sprintf("cd %s && ls -t */ | tail -n +%d | xargs -r rm -rf", r.Path, r.MaxBackups+1)
	session.Run(cmd)
}
