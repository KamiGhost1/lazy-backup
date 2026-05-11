package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	colorActive   = tcell.ColorGreen
	colorInactive = tcell.ColorDarkGray
)

var tabNames = []string{"Server", "Repo", "Logs"}

type App struct {
	db            *sql.DB
	app           *tview.Application
	pages         *tview.Pages
	serverList    *tview.List
	repoList      *tview.List
	serverInfo    *tview.TextView
	repoInfo      *tview.TextView
	logText       *tview.TextView
	tabBar        *tview.TextView
	rightPages    *tview.Pages
	helpBar       *tview.TextView
	currentTab    int
	leftFocused   int // 0=servers, 1=repos
	addServerForm *tview.Form
	addRepoForm   *tview.Form
}

func (a *App) setupUI() {
	a.serverList = tview.NewList().ShowSecondaryText(false)
	a.repoList = tview.NewList().ShowSecondaryText(false)
	a.serverInfo = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	a.repoInfo = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	a.logText = tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWordWrap(true)
	a.tabBar = tview.NewTextView().SetDynamicColors(true)
	a.helpBar = tview.NewTextView().SetDynamicColors(true)
	a.pages = tview.NewPages()
	a.rightPages = tview.NewPages()

	// Borders + titles
	a.serverList.SetBorder(true).SetTitle(" Servers ").SetTitleAlign(tview.AlignLeft)
	a.repoList.SetBorder(true).SetTitle(" Repos ").SetTitleAlign(tview.AlignLeft)
	a.serverInfo.SetBorder(true).SetTitleAlign(tview.AlignLeft)
	a.repoInfo.SetBorder(true).SetTitleAlign(tview.AlignLeft)
	a.logText.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)

	// Highlight selected item
	selStyle := tcell.StyleDefault.Background(colorActive).Foreground(tcell.ColorBlack)
	a.serverList.SetSelectedStyle(selStyle)
	a.repoList.SetSelectedStyle(selStyle)

	// When server selection changes — reload repos and refresh right panel
	a.serverList.SetChangedFunc(func(_ int, _ string, _ string, _ rune) {
		a.refreshRepoTitle()
		a.loadReposForCurrentServer()
		a.updateRightPanel()
	})
	a.repoList.SetChangedFunc(func(_ int, _ string, _ string, _ rune) {
		a.updateRightPanel()
	})

	a.rightPages.AddPage("server", a.serverInfo, true, true)
	a.rightPages.AddPage("repo", a.repoInfo, true, false)
	a.rightPages.AddPage("logs", a.logText, true, false)

	leftFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.serverList, 0, 1, true).
		AddItem(a.repoList, 0, 1, false)

	rightFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.tabBar, 1, 0, false).
		AddItem(a.rightPages, 0, 1, false)

	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().
			AddItem(leftFlex, 0, 1, true).
			AddItem(rightFlex, 0, 2, false),
			0, 1, true).
		AddItem(a.helpBar, 1, 0, false)

	a.pages.AddPage("main", mainFlex, true, true)
	a.pages.AddPage("addServer", a.createAddServerPage(), true, false)
	a.pages.AddPage("addRepo", a.createAddRepoPage(), true, false)

	a.helpBar.SetText(helpText())
	a.renderTabBar()
	a.refreshBorders()

	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		page, _ := a.pages.GetFrontPage()
		if page == "main" {
			switch event.Rune() {
			case 'a':
				if a.leftFocused == 0 {
					a.pages.SwitchToPage("addServer")
					a.app.SetFocus(a.addServerForm)
				} else {
					if a.serverList.GetItemCount() == 0 {
						a.showHelp("[red]Add a server first[white]")
						return nil
					}
					a.pages.SwitchToPage("addRepo")
					a.app.SetFocus(a.addRepoForm)
				}
				return nil
			case 'd':
				if a.leftFocused == 0 {
					a.deleteSelectedServer()
				} else {
					a.deleteSelectedRepo()
				}
				return nil
			case 'g':
				go a.performBackup("download")
				return nil
			case 'u':
				go a.performBackup("upload")
				return nil
			case 'q':
				a.app.Stop()
				return nil
			case '[':
				a.switchTab(-1)
				return nil
			case ']':
				a.switchTab(1)
				return nil
			}
			if event.Key() == tcell.KeyTab {
				a.cycleFocus()
				return nil
			}
		} else if page == "addServer" || page == "addRepo" {
			if event.Key() == tcell.KeyEsc {
				a.pages.SwitchToPage("main")
				a.helpBar.SetText(helpText())
				a.restoreFocus()
				return nil
			}
		}
		if event.Key() == tcell.KeyCtrlC {
			a.app.Stop()
			return nil
		}
		return event
	})

	a.loadServers()
	a.app.SetFocus(a.serverList)
}

// ── Focus / tabs ─────────────────────────────────────────────────────────────

func (a *App) cycleFocus() {
	a.leftFocused = (a.leftFocused + 1) % 2
	a.refreshBorders()
	if a.leftFocused == 0 {
		a.app.SetFocus(a.serverList)
	} else {
		a.app.SetFocus(a.repoList)
	}
}

func (a *App) restoreFocus() {
	a.refreshBorders()
	if a.leftFocused == 0 {
		a.app.SetFocus(a.serverList)
	} else {
		a.app.SetFocus(a.repoList)
	}
}

func (a *App) refreshBorders() {
	if a.leftFocused == 0 {
		a.serverList.SetBorderColor(colorActive).SetTitleColor(colorActive)
		a.repoList.SetBorderColor(colorInactive).SetTitleColor(tcell.ColorWhite)
	} else {
		a.serverList.SetBorderColor(colorInactive).SetTitleColor(tcell.ColorWhite)
		a.repoList.SetBorderColor(colorActive).SetTitleColor(colorActive)
	}
}

func (a *App) switchTab(delta int) {
	a.currentTab = (a.currentTab + delta + len(tabNames)) % len(tabNames)
	switch a.currentTab {
	case 0:
		a.rightPages.SwitchToPage("server")
		a.updateServerInfo()
	case 1:
		a.rightPages.SwitchToPage("repo")
		a.updateRepoInfo()
	case 2:
		a.rightPages.SwitchToPage("logs")
	}
	a.renderTabBar()
}

func (a *App) renderTabBar() {
	var sb strings.Builder
	for i, name := range tabNames {
		if i == a.currentTab {
			fmt.Fprintf(&sb, " [green::b]%s[::-]", name)
		} else {
			fmt.Fprintf(&sb, " [darkgray]%s[white]", name)
		}
	}
	a.tabBar.SetText(sb.String())
}

func helpText() string {
	return " [green]tab[white] panels  [green]a[white] add  [green]d[white] del  [green]g[white] ↓  [green]u[white] ↑  [green][[white]/[green]][white] tabs  [green]q[white] quit"
}

func (a *App) showHelp(msg string) {
	a.helpBar.SetText(" " + msg + "  [darkgray](any key to continue)[white]")
}

// ── Right panel content ───────────────────────────────────────────────────────

func (a *App) updateRightPanel() {
	switch a.currentTab {
	case 0:
		a.updateServerInfo()
	case 1:
		a.updateRepoInfo()
	}
}

func (a *App) updateServerInfo() {
	a.serverInfo.Clear()
	a.serverInfo.SetTitle(" Server ")
	if a.serverList.GetItemCount() == 0 {
		fmt.Fprint(a.serverInfo, "\n  [darkgray]No servers configured[white]")
		return
	}
	item, _ := a.serverList.GetItemText(a.serverList.GetCurrentItem())
	s, err := getServerByName(a.db, extractServerName(item))
	if err != nil {
		return
	}
	auth := "[yellow]password[white]"
	if s.KeyPath != "" {
		auth = fmt.Sprintf("[green]key[white]  %s", s.KeyPath)
	}
	fmt.Fprintf(a.serverInfo,
		"\n  [::b]Name[::] %s\n  [::b]Host[::] %s\n  [::b]Port[::] %d\n  [::b]User[::] %s\n  [::b]Auth[::] %s\n",
		s.Name, s.Host, s.Port, s.User, auth)
}

func (a *App) updateRepoInfo() {
	a.repoInfo.Clear()
	a.repoInfo.SetTitle(" Repo ")
	if a.repoList.GetItemCount() == 0 {
		fmt.Fprint(a.repoInfo, "\n  [darkgray]No repos configured[white]")
		return
	}
	fullItem, _ := a.repoList.GetItemText(a.repoList.GetCurrentItem())
	repoName := extractRepoName(fullItem)
	serverID := a.currentServerID()
	id, err := getRepoIDByNameAndServer(a.db, repoName, serverID)
	if err != nil {
		return
	}
	r, _ := getRepoByID(a.db, id)

	fmt.Fprintf(a.repoInfo,
		"\n  [::b]Name[::] %s\n  [::b]Path[::] %s\n  [::b]Max[::]  %d backups\n\n  [::b]Local backups:[::]\n",
		r.Name, r.Path, r.MaxBackups)

	prefix := fmt.Sprintf("repo%d_", r.ID)
	entries, _ := os.ReadDir("backups")
	found := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, _ := e.Info()
		size := calcDirSize(filepath.Join("backups", e.Name()))
		fmt.Fprintf(a.repoInfo, "  [green]▸[white] %-34s %6s  %s\n",
			e.Name(), fmtSize(size), info.ModTime().Format("2006-01-02 15:04"))
		found++
	}
	if found == 0 {
		fmt.Fprint(a.repoInfo, "  [darkgray]none[white]\n")
	}
}

func calcDirSize(path string) int64 {
	var n int64
	filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}

func fmtSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(1024), 0
	for n := b / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ── Forms ─────────────────────────────────────────────────────────────────────

func (a *App) createAddServerPage() tview.Primitive {
	form := tview.NewForm()
	a.addServerForm = form
	form.SetBorder(true).SetTitle(" Add Server ").
		SetBorderColor(colorActive).SetTitleColor(colorActive).SetTitleAlign(tview.AlignLeft)

	form.AddInputField("Name    ", "", 24, nil, nil)
	form.AddInputField("Host    ", "", 24, nil, nil)
	form.AddInputField("Port    ", "22", 6, nil, nil)
	form.AddInputField("User    ", "", 24, nil, nil)
	form.AddPasswordField("Password", "", 24, '*', nil)
	form.AddInputField("Key path", "", 36, nil, nil)

	form.AddButton("Save", func() {
		name := form.GetFormItem(0).(*tview.InputField).GetText()
		host := form.GetFormItem(1).(*tview.InputField).GetText()
		port := form.GetFormItem(2).(*tview.InputField).GetText()
		user := form.GetFormItem(3).(*tview.InputField).GetText()
		pass := form.GetFormItem(4).(*tview.InputField).GetText()
		key := form.GetFormItem(5).(*tview.InputField).GetText()
		if name == "" || host == "" || user == "" {
			a.showHelp("[red]Name, host and user are required[white]")
			return
		}
		if err := addServerDB(a.db, name, host, port, user, pass, key); err != nil {
			a.showHelp(fmt.Sprintf("[red]DB error: %v[white]", err))
			return
		}
		a.loadServers()
		a.log(fmt.Sprintf("[green]✓[white] Server '%s' added", name))
		a.pages.SwitchToPage("main")
		a.helpBar.SetText(helpText())
		a.leftFocused = 0
		a.restoreFocus()
	})
	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.helpBar.SetText(helpText())
		a.restoreFocus()
	})
	form.SetButtonsAlign(tview.AlignLeft)
	return centeredModal(form, 54)
}

func (a *App) createAddRepoPage() tview.Primitive {
	form := tview.NewForm()
	a.addRepoForm = form
	form.SetBorder(true).SetTitle(" Add Repo ").
		SetBorderColor(colorActive).SetTitleColor(colorActive).SetTitleAlign(tview.AlignLeft)

	form.AddInputField("Name       ", "", 24, nil, nil)
	form.AddInputField("Remote path", "", 36, nil, nil)
	form.AddInputField("Max backups", "5", 6, nil, nil)

	form.AddButton("Save", func() {
		name := form.GetFormItem(0).(*tview.InputField).GetText()
		path := form.GetFormItem(1).(*tview.InputField).GetText()
		maxStr := form.GetFormItem(2).(*tview.InputField).GetText()
		maxBackups, _ := strconv.Atoi(maxStr)
		if maxBackups <= 0 {
			maxBackups = 5
		}
		if name == "" || path == "" {
			a.showHelp("[red]Name and path are required[white]")
			return
		}
		serverID := a.currentServerID()
		if serverID <= 0 {
			a.showHelp("[red]No server selected[white]")
			return
		}
		if err := addRepoDB(a.db, name, path, serverID, maxBackups); err != nil {
			a.showHelp(fmt.Sprintf("[red]DB error: %v[white]", err))
			return
		}
		a.loadRepos(serverID)
		a.log(fmt.Sprintf("[green]✓[white] Repo '%s' added", name))
		a.pages.SwitchToPage("main")
		a.helpBar.SetText(helpText())
		a.leftFocused = 1
		a.restoreFocus()
	})
	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.helpBar.SetText(helpText())
		a.restoreFocus()
	})
	form.SetButtonsAlign(tview.AlignLeft)
	return centeredModal(form, 58)
}

// centeredModal places a widget in the center with fixed width, flexible height.
func centeredModal(p tview.Primitive, w int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, 0, 3, true).
			AddItem(nil, 0, 1, false), w, 0, true).
		AddItem(nil, 0, 1, false)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (a *App) currentServerID() int {
	if a.serverList.GetItemCount() == 0 {
		return 0
	}
	item, _ := a.serverList.GetItemText(a.serverList.GetCurrentItem())
	s, err := getServerByName(a.db, extractServerName(item))
	if err != nil {
		return 0
	}
	return s.ID
}

func (a *App) refreshRepoTitle() {
	if a.serverList.GetItemCount() == 0 {
		a.repoList.SetTitle(" Repos ")
		return
	}
	item, _ := a.serverList.GetItemText(a.serverList.GetCurrentItem())
	name := extractServerName(item)
	a.repoList.SetTitle(fmt.Sprintf(" Repos [%s] ", name))
}

func (a *App) loadServers() {
	a.serverList.Clear()
	servers, err := loadServers(a.db)
	if err != nil {
		a.log(fmt.Sprintf("[red]%v[white]", err))
		return
	}
	for _, s := range servers {
		a.serverList.AddItem(formatServerItem(s), "", 0, nil)
	}
	if len(servers) > 0 {
		a.serverList.SetCurrentItem(0)
		a.loadRepos(servers[0].ID)
	}
	a.refreshRepoTitle()
	a.updateRightPanel()
}

func (a *App) loadRepos(serverID int) {
	a.repoList.Clear()
	repos, _ := loadReposForServer(a.db, serverID)
	for _, r := range repos {
		a.repoList.AddItem(formatRepoItem(r), "", 0, nil)
	}
}

func (a *App) loadReposForCurrentServer() {
	sid := a.currentServerID()
	if sid > 0 {
		a.loadRepos(sid)
	}
}

func (a *App) deleteSelectedServer() {
	if a.serverList.GetItemCount() == 0 {
		return
	}
	item, _ := a.serverList.GetItemText(a.serverList.GetCurrentItem())
	s, err := getServerByName(a.db, extractServerName(item))
	if err != nil {
		a.log(fmt.Sprintf("[red]%v[white]", err))
		return
	}
	repos, _ := loadReposForServer(a.db, s.ID)
	for _, r := range repos {
		deleteRepoDB(a.db, r.ID)
	}
	if err := deleteServerDB(a.db, s.ID); err != nil {
		a.log(fmt.Sprintf("[red]%v[white]", err))
		return
	}
	a.loadServers()
	a.log(fmt.Sprintf("[yellow]–[white] Server '%s' removed", s.Name))
}

func (a *App) deleteSelectedRepo() {
	if a.repoList.GetItemCount() == 0 {
		return
	}
	fullItem, _ := a.repoList.GetItemText(a.repoList.GetCurrentItem())
	repoName := extractRepoName(fullItem)
	serverID := a.currentServerID()
	id, err := getRepoIDByNameAndServer(a.db, repoName, serverID)
	if err != nil {
		a.log(fmt.Sprintf("[red]Repo not found: %v[white]", err))
		return
	}
	if err := deleteRepoDB(a.db, id); err != nil {
		a.log(fmt.Sprintf("[red]%v[white]", err))
		return
	}
	a.loadRepos(serverID)
	a.log(fmt.Sprintf("[yellow]–[white] Repo '%s' removed", repoName))
}

// ── Backup ────────────────────────────────────────────────────────────────────

func (a *App) performBackup(direction string) {
	serverID := a.currentServerID()
	if serverID == 0 || a.repoList.GetItemCount() == 0 {
		a.app.QueueUpdateDraw(func() { a.showHelp("[red]Select a server and repo first[white]") })
		return
	}
	fullItem, _ := a.repoList.GetItemText(a.repoList.GetCurrentItem())
	repoName := extractRepoName(fullItem)

	serverItem, _ := a.serverList.GetItemText(a.serverList.GetCurrentItem())
	s, err := getServerByName(a.db, extractServerName(serverItem))
	if err != nil {
		a.app.QueueUpdateDraw(func() { a.showHelp(fmt.Sprintf("[red]%v[white]", err)) })
		return
	}
	id, err := getRepoIDByNameAndServer(a.db, repoName, s.ID)
	if err != nil {
		a.app.QueueUpdateDraw(func() { a.showHelp(fmt.Sprintf("[red]%v[white]", err)) })
		return
	}
	r, _ := getRepoByID(a.db, id)

	arrow := "↓"
	if direction == "upload" {
		arrow = "↑"
	}
	a.app.QueueUpdateDraw(func() {
		a.helpBar.SetText(fmt.Sprintf(" [yellow]%s %s %s…[white]", arrow, direction, repoName))
	})

	os.MkdirAll("backups", 0755)
	var status, details string
	if direction == "download" {
		msg, err := downloadBackup(s, r, "backups")
		if err != nil {
			status, details = "failed", err.Error()
		} else {
			status, details = "completed", msg
		}
	} else {
		msg, err := uploadBackup(s, r, "backups")
		if err != nil {
			status, details = "failed", err.Error()
		} else {
			status, details = "completed", msg
		}
	}

	icon := "[green]✓[white]"
	if status == "failed" {
		icon = "[red]✗[white]"
	}
	a.app.QueueUpdateDraw(func() {
		a.logText.Write([]byte(fmt.Sprintf("[darkgray]%s[white] %s %s %s: %s\n",
			time.Now().Format("15:04:05"), icon, arrow, repoName, details)))
		a.helpBar.SetText(helpText())
		if a.currentTab == 1 {
			a.updateRepoInfo()
		}
	})
	insertBackupLog(a.db, r.ID, direction, status, details)
}

// log writes directly to logText — must only be called from the main tview goroutine.
func (a *App) log(msg string) {
	if a.logText == nil {
		return
	}
	a.logText.Write([]byte(fmt.Sprintf("[darkgray]%s[white] %s\n",
		time.Now().Format("15:04:05"), msg)))
}
