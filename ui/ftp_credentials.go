package ui

import (
	"fmt"
	"sync"

	"filecompare/core"
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ftpSessionCache holds FTP passwords entered by the user for the lifetime
// of the running application. It is intentionally process-memory-only and
// is never written to config.json or any other file.
var (
	ftpSessionMu    sync.Mutex
	ftpSessionCache = map[core.FTPCredentialKey]string{}
)

// clearFTPSessionCache discards every cached FTP password. Call this when
// the main window closes so passwords don't outlive it.
func clearFTPSessionCache() {
	ftpSessionMu.Lock()
	defer ftpSessionMu.Unlock()
	for k := range ftpSessionCache {
		delete(ftpSessionCache, k)
	}
}

func getCachedFTPPassword(key core.FTPCredentialKey) (string, bool) {
	ftpSessionMu.Lock()
	defer ftpSessionMu.Unlock()
	pw, ok := ftpSessionCache[key]
	return pw, ok
}

func setCachedFTPPassword(key core.FTPCredentialKey, password string) {
	ftpSessionMu.Lock()
	defer ftpSessionMu.Unlock()
	ftpSessionCache[key] = password
}

func clearCachedFTPPassword(key core.FTPCredentialKey) {
	ftpSessionMu.Lock()
	defer ftpSessionMu.Unlock()
	delete(ftpSessionCache, key)
}

// ftpServerKey identifies a server by domain+port only (no username), used
// to share a single login prompt across path entries that point at the same
// server but don't have an account configured yet.
type ftpServerKey struct {
	domain string
	port   int
}

// resolveFTPCredentials ensures a verified account+password is available for
// every distinct FTP server referenced by project's path entries, prompting
// the user where needed. It returns an updated copy of project where any
// blank PathEntry.FTPUsername has been filled in with whatever account was
// entered (or already configured), so that Compare's real connections log in
// as the exact same account the password was verified against.
//
// This must be called from a background goroutine, not the Fyne UI/driver
// goroutine: it blocks waiting for dialog responses, and those dialogs'
// callbacks are themselves dispatched on the UI goroutine, so calling this
// from that same goroutine would deadlock.
func resolveFTPCredentials(win fyne.Window, project models.Project) (models.Project, core.FTPCredentials, bool) {
	creds := core.FTPCredentials{}
	resolved := project
	resolved.Paths = make([]models.PathEntry, len(project.Paths))
	copy(resolved.Paths, project.Paths)

	resolvedUsernames := map[ftpServerKey]string{}
	seen := map[core.FTPCredentialKey]bool{}

	for i, pe := range resolved.Paths {
		if !pe.Type.IsFTP() {
			continue
		}

		sk := ftpServerKey{domain: pe.FTPDomain, port: pe.Port()}
		if pe.FTPUsername == "" {
			if u, ok := resolvedUsernames[sk]; ok {
				pe.FTPUsername = u
				resolved.Paths[i].FTPUsername = u
			}
		}

		key := core.FTPKey(pe)
		if seen[key] {
			continue
		}
		seen[key] = true

		username, password, ok := resolveOneFTPCredential(win, pe)
		if !ok {
			return models.Project{}, nil, false
		}

		resolved.Paths[i].FTPUsername = username
		resolvedUsernames[sk] = username
		creds[core.FTPCredentialKey{Domain: pe.FTPDomain, Port: pe.Port(), Username: username}] = password
	}

	return resolved, creds, true
}

// resolveOneFTPCredential returns a verified (username, password) for pe's
// FTP server. If pe.FTPUsername is already set, a cached password for it is
// revalidated before being reused; if that fails, or nothing is cached yet,
// the user is prompted (looping on login failure) until it succeeds or they
// cancel. If pe.FTPUsername is blank, the user is prompted for both an
// account and a password, since no account was configured for this server.
func resolveOneFTPCredential(win fyne.Window, pe models.PathEntry) (string, string, bool) {
	needsUsername := pe.FTPUsername == ""

	if !needsUsername {
		key := core.FTPKey(pe)
		if cached, ok := getCachedFTPPassword(key); ok {
			if verifyFTPLogin(pe, cached) {
				return pe.FTPUsername, cached, true
			}
			clearCachedFTPPassword(key)
		}
	}

	for {
		username, password, ok := showFTPLoginDialog(win, pe, needsUsername)
		if !ok {
			return "", "", false
		}

		attempt := pe
		attempt.FTPUsername = username
		if verifyFTPLogin(attempt, password) {
			setCachedFTPPassword(core.FTPKey(attempt), password)
			return username, password, true
		}

		showBlockingInfo(win, "Login Failed",
			fmt.Sprintf("Could not log in to %s as \"%s\". Please check the account and password and try again.", pe.FTPDomain, username))
	}
}

// verifyFTPLogin performs a throwaway connection to confirm the password
// works, independent of the connection later used for the actual scan.
func verifyFTPLogin(pe models.PathEntry, password string) bool {
	conn, err := core.DialFTP(pe, password)
	if err != nil {
		return false
	}
	_ = conn.Quit()
	return true
}

// showFTPLoginDialog blocks until the user submits or cancels the login
// form, returning the account, the password, and whether they confirmed.
// When editableUsername is false, the account field shows pe.FTPUsername
// read-only (it's already configured on the server profile); when true, no
// account was configured for this server, so the field is editable and the
// user is expected to fill in both the account and the password.
func showFTPLoginDialog(win fyne.Window, pe models.PathEntry, editableUsername bool) (string, string, bool) {
	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(pe.FTPUsername)
	if editableUsername {
		usernameEntry.SetPlaceHolder("Account")
	} else {
		usernameEntry.Disable()
	}
	passwordEntry := widget.NewPasswordEntry()

	form := widget.NewForm(
		widget.NewFormItem("Account", usernameEntry),
		widget.NewFormItem("Password", passwordEntry),
	)

	resultCh := make(chan bool, 1)
	title := fmt.Sprintf("Log in to %s (%s)", pe.FTPDomain, pe.Label)
	d := NewStyledPopup(win, title, form, []PopupButton{
		{Text: "Cancel", OnTapped: func() { resultCh <- false }},
		{Text: "Log In", Importance: widget.HighImportance, OnTapped: func() { resultCh <- true }},
	})
	d.Resize(fyne.NewSize(420, 220))
	d.Show()

	confirmed := <-resultCh
	return usernameEntry.Text, passwordEntry.Text, confirmed
}

// showBlockingInfo shows an information dialog and waits for it to be
// dismissed before returning.
func showBlockingInfo(win fyne.Window, title, message string) {
	doneCh := make(chan struct{}, 1)
	body := widget.NewLabel(message)
	body.Wrapping = fyne.TextWrapWord

	d := NewStyledPopup(win, title, body, []PopupButton{{Text: "OK"}})
	d.Resize(sizeForMessage(message))
	d.SetOnClosed(func() { doneCh <- struct{}{} })
	d.Show()
	<-doneCh
}
