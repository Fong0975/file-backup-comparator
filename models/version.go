package models

import "encoding/json"

// VersionInfo holds the build metadata shown in the app's About tab.
type VersionInfo struct {
	ProductName     string
	ProductVersion  string
	FileDescription string
	FileVersion     string
	CompanyName     string
	LegalCopyright  string
}

// versionInfoFile mirrors the subset of versioninfo.json's schema (the file
// goversioninfo reads to build icon_windows_amd64.syso) needed for display.
type versionInfoFile struct {
	StringFileInfo struct {
		CompanyName     string `json:"CompanyName"`
		FileDescription string `json:"FileDescription"`
		FileVersion     string `json:"FileVersion"`
		LegalCopyright  string `json:"LegalCopyright"`
		ProductName     string `json:"ProductName"`
		ProductVersion  string `json:"ProductVersion"`
	} `json:"StringFileInfo"`
}

// ParseVersionInfo extracts the About-tab fields from the raw contents of
// versioninfo.json, so editing that one file keeps both the .syso and the
// in-app display in sync. Malformed input yields a zero-value VersionInfo
// rather than an error, since this is purely informational.
func ParseVersionInfo(data []byte) VersionInfo {
	var raw versionInfoFile
	_ = json.Unmarshal(data, &raw)
	return VersionInfo{
		ProductName:     raw.StringFileInfo.ProductName,
		ProductVersion:  raw.StringFileInfo.ProductVersion,
		FileDescription: raw.StringFileInfo.FileDescription,
		FileVersion:     raw.StringFileInfo.FileVersion,
		CompanyName:     raw.StringFileInfo.CompanyName,
		LegalCopyright:  raw.StringFileInfo.LegalCopyright,
	}
}
