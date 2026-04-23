package backend

import (
	"fmt"
	"strings"

	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

const virtualFolderAllMailOnlyName = "All Mail only"
const allMailOnlySupportedReason = "Read-only: messages in All Mail with no other folder assignment."

func findAllMailFolder(folders []string, vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "gmail":
		for _, folder := range folders {
			if folder == "[Gmail]/All Mail" {
				return folder
			}
		}
	}
	for _, folder := range folders {
		if folder == "All Mail" {
			return folder
		}
	}
	return ""
}

func buildAllMailOnlyView(allMailFolder string, allMailEmails []*models.EmailData, membershipByFolder map[string]map[string]bool, complete bool, reason string) *models.VirtualFolderResult {
	result := &models.VirtualFolderResult{
		Name:         virtualFolderAllMailOnlyName,
		Supported:    false,
		Reason:       reason,
		SourceFolder: allMailFolder,
		Emails:       []*models.EmailData{},
	}
	if allMailFolder == "" {
		if result.Reason == "" {
			result.Reason = "Provider does not expose All Mail"
		}
		return result
	}
	if !complete || membershipByFolder == nil {
		if result.Reason == "" {
			result.Reason = "Could not inspect folder membership safely"
		}
		return result
	}

	excluded := make(map[string]bool)
	for folder, ids := range membershipByFolder {
		if folder == allMailFolder {
			continue
		}
		for id := range ids {
			excluded[id] = true
		}
	}

	result.Supported = true
	result.Reason = allMailOnlySupportedReason
	for _, email := range allMailEmails {
		if email == nil || strings.TrimSpace(email.MessageID) == "" {
			continue
		}
		if excluded[email.MessageID] {
			continue
		}
		result.Emails = append(result.Emails, email)
	}
	return result
}

func (b *LocalBackend) GetAllMailOnlyView() (*models.VirtualFolderResult, error) {
	folders, err := b.imapClient.ListFolders()
	if err != nil {
		return &models.VirtualFolderResult{
			Name:      virtualFolderAllMailOnlyName,
			Supported: false,
			Reason:    fmt.Sprintf("Failed to list folders: %v", err),
			Emails:    []*models.EmailData{},
		}, nil
	}

	allMailFolder := findAllMailFolder(folders, b.cfg.Vendor)
	if allMailFolder == "" {
		return buildAllMailOnlyView("", nil, nil, true, ""), nil
	}

	if err := b.imapClient.ProcessEmailsIncremental(allMailFolder); err != nil {
		return &models.VirtualFolderResult{
			Name:         virtualFolderAllMailOnlyName,
			Supported:    false,
			Reason:       fmt.Sprintf("Failed to refresh %s metadata: %v", allMailFolder, err),
			SourceFolder: allMailFolder,
			Emails:       []*models.EmailData{},
		}, nil
	}

	membershipByFolder, err := b.imapClient.GetFolderMessageIDs(folders)
	if err != nil {
		logger.Warn("GetAllMailOnlyView: membership inspection failed: %v", err)
		return buildAllMailOnlyView(allMailFolder, nil, nil, false, fmt.Sprintf("Failed to inspect folder membership: %v", err)), nil
	}

	allMailEmails, err := b.cache.GetEmailsSortedByDate(allMailFolder)
	if err != nil {
		return nil, err
	}
	return buildAllMailOnlyView(allMailFolder, allMailEmails, membershipByFolder, true, ""), nil
}
