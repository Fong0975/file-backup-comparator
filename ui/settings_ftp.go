package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildFTPSettingsTab builds the "FTP" tab: an inline add/edit form above a
// list of saved server profiles, each with edit/delete buttons. Editing a
// row populates the form in place rather than opening another dialog.
func buildFTPSettingsTab(servers *[]models.FTPServer) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("e.g. Home NAS")
	domainEntry := widget.NewEntry()
	domainEntry.SetPlaceHolder("e.g. ftp.example.com")
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("21")
	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("optional")

	var serverList *widget.List
	editingIdx := -1

	var saveBtn *widget.Button
	resetForm := func() {
		editingIdx = -1
		nameEntry.SetText("")
		domainEntry.SetText("")
		portEntry.SetText("")
		usernameEntry.SetText("")
		saveBtn.SetText("Add")
	}

	saveBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), nil)
	saveBtn.OnTapped = func() {
		domain := strings.TrimSpace(domainEntry.Text)
		if domain == "" {
			return
		}
		name := strings.TrimSpace(nameEntry.Text)
		port, _ := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		username := strings.TrimSpace(usernameEntry.Text)

		if editingIdx >= 0 {
			(*servers)[editingIdx].Name = name
			(*servers)[editingIdx].Domain = domain
			(*servers)[editingIdx].Port = port
			(*servers)[editingIdx].Username = username
		} else {
			*servers = append(*servers, models.FTPServer{
				ID:       fmt.Sprintf("%d", time.Now().UnixNano()),
				Name:     name,
				Domain:   domain,
				Port:     port,
				Username: username,
			})
		}
		serverList.Refresh()
		resetForm()
	}
	cancelEditBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
	cancelEditBtn.OnTapped = func() { resetForm() }

	editForm := widget.NewForm(
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Domain", domainEntry),
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("Account (optional)", usernameEntry),
	)

	serverList = widget.NewList(
		func() int { return len(*servers) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("")
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			detailLabel := widget.NewLabel("")
			detailLabel.Truncation = fyne.TextTruncateEllipsis

			return container.NewBorder(
				nil, nil, nil,
				container.NewHBox(
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {}),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {}),
				),
				container.NewGridWithColumns(2, nameLabel, detailLabel),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			border := obj.(*fyne.Container)
			grid := border.Objects[0].(*fyne.Container)
			btnBox := border.Objects[1].(*fyne.Container)

			s := (*servers)[id]
			grid.Objects[0].(*widget.Label).SetText(s.DisplayName())
			account := s.Username
			if account == "" {
				account = "ask at compare time"
			}
			grid.Objects[1].(*widget.Label).SetText(fmt.Sprintf("%s:%d — %s", s.Domain, s.PortOrDefault(), account))

			idx := int(id)
			btnBox.Objects[0].(*widget.Button).OnTapped = func() {
				editingIdx = idx
				cur := (*servers)[idx]
				nameEntry.SetText(cur.Name)
				domainEntry.SetText(cur.Domain)
				if cur.Port > 0 {
					portEntry.SetText(strconv.Itoa(cur.Port))
				} else {
					portEntry.SetText("")
				}
				usernameEntry.SetText(cur.Username)
				saveBtn.SetText("Update")
			}
			btnBox.Objects[1].(*widget.Button).OnTapped = func() {
				*servers = append((*servers)[:idx], (*servers)[idx+1:]...)
				serverList.Refresh()
				if editingIdx == idx {
					resetForm()
				}
			}
		},
	)

	serverList.HideSeparators = true

	content := container.NewBorder(
		container.NewVBox(
			editForm,
			container.NewHBox(layout.NewSpacer(), cancelEditBtn, saveBtn),
			settingsFormListGap(),
			widget.NewLabelWithStyle("Saved Servers", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil, nil, nil,
		serverList,
	)
	return container.New(layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0), content)
}
