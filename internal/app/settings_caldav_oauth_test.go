package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testcalendar"
)

func TestSettingsCalDAVSaveSkipsGoogleOAuth(t *testing.T) {
	for _, provider := range []string{"caldav", "fastmail", "icloud", "yahoo"} {
		for _, editExisting := range []bool{false, true} {
			name := provider + "/add"
			if editExisting {
				name = provider + "/edit"
			}
			t.Run(name, func(t *testing.T) {
				// Keep an unrelated Google account to prove authorization stays
				// scoped to the account being saved.
				existing := &config.Config{Sources: []config.SourceConfig{{
					ID: "google-mail", Kind: "mail", Provider: "gmail_api", AccountID: "google",
					Google: config.GoogleConfig{Email: "google@example.test"},
				}}}
				if editExisting {
					existing.Sources = append(existing.Sources, config.SourceConfig{
						ID: "personal-calendar", Kind: "calendar", Provider: "caldav", AccountID: "personal",
						CalDAV: config.CalDAVConfig{URL: "https://caldav.example.test/calendar", Username: "me@example.test", Password: "old-password"},
					})
				}
				s := NewSettings(SettingsModePanel, existing)
				s.panelSection = settingsPanelSectionAccounts
				s.accountsMenuChoice = settingsAccountEditAddCalendar
				if editExisting {
					s.accountsMenuChoice = "account:personal"
				}
				s.buildForm()
				s, _ = updateAndPumpSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEnter})
				s.calendarProvider = provider
				s.calendarEmail = "me@example.test"
				s.caldavURL = "https://caldav.example.test/calendar"
				s.caldavUsername = "me@example.test"
				s.caldavPassword = "new-password"
				if provider == "caldav" {
					lab := testcalendar.Start(t, testcalendar.WithCalendar("personal", "Personal", "#0b8043"))
					s.caldavURL = lab.CalDAVSourceConfig("personal", "personal").CalDAV.URL
				}
				s.buildForm()
				// Exercise the actual completed-form dispatch, not a hand-built
				// SettingsSavedMsg, which would bypass the OAuth decision.
				s.form.State = huh.StateCompleted
				_, messages := updateAndPumpSettingsForTest(t, s, nil)
				var saved *SettingsSavedMsg
				for _, message := range messages {
					switch msg := message.(type) {
					case OAuthRequiredMsg:
						t.Fatalf("CalDAV save requested Google OAuth for sources %v", msg.SourceIDs)
					case SettingsSavedMsg:
						saved = &msg
					}
				}
				if saved == nil || saved.Config == nil {
					t.Fatal("CalDAV save did not emit SettingsSavedMsg")
				}
				if !saved.ValidateCalendar || saved.ValidateAccount || len(saved.CalendarSourceIDs) != 1 {
					t.Fatalf("expected only scoped calendar validation, got calendar=%v account=%v sources=%v", saved.ValidateCalendar, saved.ValidateAccount, saved.CalendarSourceIDs)
				}
				calendar := settingsTestSourceByID(t, saved.Config.Sources, string(saved.CalendarSourceIDs[0]))
				if calendar.Provider != "caldav" || calendar.CalDAV.URL != s.caldavURL || calendar.CalDAV.Username != s.caldavUsername || calendar.CalDAV.Password != s.caldavPassword {
					t.Fatal("CalDAV credentials were not preserved in the candidate")
				}
				google := settingsTestSourceByID(t, saved.Config.Sources, "google-mail")
				if google.Google.Email != "google@example.test" {
					t.Fatal("unrelated Google account was changed")
				}
				if provider == "caldav" {
					result := validateCalendarSettingsCmd(*saved)().(CalendarValidationMsg)
					if result.Err != nil {
						t.Fatalf("saved CalDAV source failed local provider validation: %v", result.Err)
					}
				}
			})
		}
	}
}
