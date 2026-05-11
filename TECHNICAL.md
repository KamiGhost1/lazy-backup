# Техническое описание проекта

## Обзор архитектуры

Приложение написано на Go и состоит из двух режимов работы — TUI и CLI — с общим слоем бизнес-логики и хранилища.

```
main.go
  ├── args > 0 → runCLI()   (cli.go)
  └── args == 0 → TUI App   (ui.go)
           │
           ├── db layer      (db.go)     — CRUD поверх SQLite
           ├── ssh/sftp ops  (sshutils.go) — перенос файлов
           └── data models   (models.go)
```

## Структура файлов

| Файл | Назначение |
|------|-----------|
| `main.go` | Точка входа, выбор режима TUI/CLI |
| `models.go` | Структуры данных: `Server`, `BackupRepo`, `BackupLog` |
| `db.go` | Инициализация SQLite, все CRUD-операции |
| `ui.go` | TUI-интерфейс на tview, структура `App` |
| `cli.go` | CLI-парсер флагов (`flag`), обёртка над бизнес-логикой |
| `sshutils.go` | SSH-соединение, rsync/SFTP-трансфер, ротация, генерация ключей |
| `db_test.go` | Unit-тесты слоя БД |
| `sshutils_test.go` | Unit-тесты ротации и генерации ключей |

## Модели данных

```go
Server {
    ID, Name, Host string
    Port     int       // default 22
    User     string
    Password string    // опционально
    KeyPath  string    // опционально, путь к RSA-ключу
}

BackupRepo {
    ID, ServerID int
    Name, Path   string
    MaxBackups   int   // лимит хранимых копий
}

BackupLog {
    ID, RepoID  int
    Direction   string    // "download" | "upload"
    Status      string    // "completed" | "failed"
    Timestamp   time.Time
    Details     string
}
```

## База данных

Используется SQLite через `modernc.org/sqlite` (pure-Go, CGO не требуется). Файл: `backup_manager.db` в рабочей директории.

### Схема

```sql
servers (id, name, host, port, user, password, key_path)
backup_repos (id, server_id FK→servers, path, name, max_backups)
backup_logs (id, repo_id FK→backup_repos, direction, status, timestamp, details)
```

Таблицы создаются при старте (`CREATE TABLE IF NOT EXISTS`).

## TUI-слой (ui.go)

Построен на `tview` + `tcell`. Структура `App` хранит все виджеты и ссылку на `*sql.DB`.

### Страницы (tview.Pages)

| Страница | Содержимое |
|----------|-----------|
| `main` | Трёхколоночный Flex: серверы / репозитории / логи + хелпбар |
| `addServer` | Центрированная форма добавления сервера |
| `addRepo` | Центрированная форма добавления репозитория |

### Поток событий

```
SetInputCapture (глобальный)
  ├── 'a' → SwitchToPage("addServer" | "addRepo")
  ├── 'd' → deleteSelectedServer() | deleteSelectedRepo()
  ├── 'g' → go performBackup("download")   ← горутина
  ├── 'u' → go performBackup("upload")     ← горутина
  ├── 'q' → app.Stop()
  ├── Tab → смена фокуса между панелями
  └── Esc (на форме) → SwitchToPage("main")
```

Операции бэкапа выполняются в горутине, UI-обновления идут через `app.QueueUpdateDraw`.

## SSH / передача файлов (sshutils.go)

### Выбор метода передачи

```
rsyncAvailable() == true  →  copyUsingRsync()   (быстро, инкрементально)
rsyncAvailable() == false →  copyUsingSFTP()    (fallback, через pkg/sftp)
```

### Аутентификация SSH

`connectSSH` поддерживает два метода (можно одновременно):
- Пароль: `ssh.Password()`
- Приватный ключ: `ssh.PublicKeys()` (RSA PEM)

> `HostKeyCallback: ssh.InsecureIgnoreHostKey()` — проверка хоста отключена (упрощение для LAN-окружений).

### rsync

```bash
# download
rsync -avz user@host:/remote/path /local/path

# upload
rsync -avz /local/path user@host:/remote/path
```

SSH-ключ передаётся через переменную окружения `RSYNC_RSH`, пароль — через `RSYNC_PASSWORD`.

### SFTP-fallback

`sftpDownload` и `sftpUpload` работают с одиночным файлом через `io.Copy`. Для директорий текущая реализация копирует только корневой объект — рекурсивный обход не реализован.

### Ротация бэкапов

**Локальная** (`rotateLocalBackups`):
1. Найти все директории с префиксом `repo{ID}_` в `backups/`
2. Отсортировать по `ModTime`
3. Удалить (`os.RemoveAll`) старейшие, оставив `MaxBackups` штук

**Удалённая** (`rotateRemoteBackups`):
- Запускается в отдельной горутине после `uploadBackup`
- Выполняет SSH-команду: `cd <path> && ls -t */ | tail -n +N | xargs -r rm -rf`

### Генерация ключей

`generateSSHKeyPair(keyPath)` создаёт RSA 2048-бит пару:
- `<keyPath>` — приватный ключ в PEM (PKCS#1), права `0600`
- `<keyPath>.pub` — публичный ключ в формате `authorized_keys`

## CLI-режим (cli.go)

Разбирает флаги через стандартный пакет `flag`:

```
-list-servers              список серверов
-list-repos -server=<id>   список репозиториев сервера
-repo=<id> -action=download|upload  выполнить операцию
-show-pubkey=<server_id>   вывести публичный ключ
```

После операции пишет запись в `backup_logs`.

## Docker-окружение

`docker-compose.yml` поднимает контейнер разработки на образе `golang:1.25.3` с установленным `docker.io`. Монтирует:
- `./app` → `/app` — исходный код
- `/var/run/docker.sock` → сокет Docker (для запуска контейнеров изнутри)
- `.vscode_extensions` → VS Code Server (поддержка Remote Development)

Контейнер запускается с `stdin_open: true` + `tty: true` для интерактивной работы.

Кросс-компиляция настраивается через переменные окружения в Dockerfile (закомментированы):
```dockerfile
# ENV GOOS=windows|linux|darwin
# ENV GOARCH=amd64|arm64
```

## Тесты

### db_test.go
- `TestAddAndGetServer` — добавление и чтение сервера
- `TestAddAndGetRepo` — добавление репозитория и проверка `max_backups`
- `TestFormatFunctions` — форматирование строк для TUI-списков

Используется in-memory SQLite (`sqlite3 :memory:`). Обратите внимание: тест использует драйвер `sqlite3` (CGO), тогда как приложение использует `sqlite` (pure-Go) — потенциальное расхождение в окружениях без CGO.

### sshutils_test.go
- `TestRotateLocalBackups` — создаёт 5 папок с разными `mtime`, проверяет, что после ротации остаётся 3
- `TestGenerateAndReadPublicKey` — генерирует ключевую пару, читает публичный ключ

## Известные ограничения

| Ограничение | Описание |
|-------------|----------|
| SFTP-fallback | Копирует только одиночный файл/директорию без рекурсии |
| Host key check | `InsecureIgnoreHostKey` — уязвимо к MITM, приемлемо только в доверенных сетях |
| Пароли в БД | Хранятся в открытом виде в SQLite |
| Удалённая ротация | Зависит от наличия `ls`, `tail`, `xargs`, `rm` на удалённом хосте |
| Тест-драйвер | `db_test.go` использует `mattn/go-sqlite3` (CGO), отличный от production-драйвера |
