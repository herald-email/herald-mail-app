package backend

import (
	"fmt"
	"time"

	"mail-processor/internal/models"
)

// seedDemoContacts returns synthetic contacts matching the demo email senders.
func seedDemoContacts() []models.ContactData {
	now := time.Now()
	return []models.ContactData{
		{
			ID:          1,
			Email:       "newsletter@techweekly.example",
			DisplayName: "Tech Weekly",
			Company:     "TechWeekly Media",
			Topics:      []string{"technology", "news", "weekly digest"},
			FirstSeen:   now.AddDate(0, -6, 0),
			LastSeen:    now.AddDate(0, 0, -1),
			EmailCount:  8,
			SentCount:   0,
		},
		{
			ID:          2,
			Email:       "orders@shopify-brand.example",
			DisplayName: "ShopifyBrand",
			Company:     "ShopifyBrand Store",
			Topics:      []string{"ecommerce", "orders", "shipping"},
			FirstSeen:   now.AddDate(0, -4, 0),
			LastSeen:    now.AddDate(0, 0, -3),
			EmailCount:  8,
			SentCount:   0,
		},
		{
			ID:          3,
			Email:       "noreply@twitter-x.example",
			DisplayName: "Twitter/X",
			Company:     "X Corp",
			Topics:      []string{"social media", "notifications"},
			FirstSeen:   now.AddDate(0, -3, 0),
			LastSeen:    now.AddDate(0, 0, -5),
			EmailCount:  6,
			SentCount:   0,
		},
		{
			ID:          4,
			Email:       "billing@aws-demo.example",
			DisplayName: "AWS",
			Company:     "Amazon Web Services",
			Topics:      []string{"cloud", "billing", "infrastructure"},
			FirstSeen:   now.AddDate(0, -12, 0),
			LastSeen:    now.AddDate(0, 0, -2),
			EmailCount:  6,
			SentCount:   0,
		},
		{
			ID:          5,
			Email:       "dr.jones@healthclinic.example",
			DisplayName: "Dr. Jones",
			Company:     "Health Clinic",
			Topics:      []string{"healthcare", "appointments"},
			FirstSeen:   now.AddDate(0, -2, 0),
			LastSeen:    now.AddDate(0, 0, -7),
			EmailCount:  6,
			SentCount:   2,
		},
		{
			ID:          6,
			Email:       "deals@airbnb-promo.example",
			DisplayName: "Airbnb",
			Company:     "Airbnb Inc.",
			Topics:      []string{"travel", "accommodation", "promotions"},
			FirstSeen:   now.AddDate(0, -5, 0),
			LastSeen:    now.AddDate(0, 0, -4),
			EmailCount:  6,
			SentCount:   0,
		},
		{
			ID:          7,
			Email:       "github-noreply@github-demo.example",
			DisplayName: "GitHub",
			Company:     "GitHub / Microsoft",
			Topics:      []string{"development", "code review", "pull requests"},
			FirstSeen:   now.AddDate(0, -8, 0),
			LastSeen:    now.AddDate(0, 0, -1),
			EmailCount:  6,
			SentCount:   0,
		},
		{
			ID:          8,
			Email:       "statements@demobank.example",
			DisplayName: "Demo Bank",
			Company:     "Demo Bank N.A.",
			Topics:      []string{"finance", "banking", "statements"},
			FirstSeen:   now.AddDate(0, -24, 0),
			LastSeen:    now.AddDate(0, 0, -6),
			EmailCount:  6,
			SentCount:   1,
		},
	}
}

var demoSenders = []struct {
	addr    string
	name    string
	folder  string
	subject string
}{
	{"newsletter@techweekly.example", "Tech Weekly", "INBOX", "This Week in Tech"},
	{"orders@shopify-brand.example", "ShopifyBrand", "Receipts", "Your order has shipped"},
	{"noreply@twitter-x.example", "Twitter/X", "Social", "You have new mentions"},
	{"billing@aws-demo.example", "AWS", "INBOX", "Your AWS bill is ready"},
	{"dr.jones@healthclinic.example", "Dr. Jones", "INBOX", "Appointment reminder"},
	{"deals@airbnb-promo.example", "Airbnb", "INBOX", "Weekend deals near you"},
	{"github-noreply@github-demo.example", "GitHub", "INBOX", "Pull request review requested"},
	{"statements@demobank.example", "Demo Bank", "INBOX", "Your statement is ready"},
}

func seedDemoEmails() []*models.EmailData {
	var emails []*models.EmailData
	now := time.Now()
	id := 1
	for i, s := range demoSenders {
		count := 6 // 6 emails per sender = 48 + a few extras
		if i < 2 {
			count = 8
		}
		for j := 0; j < count; j++ {
			emails = append(emails, &models.EmailData{
				MessageID: fmt.Sprintf("demo-%d@demo.local", id),
				UID:       uint32(id),
				Sender:    fmt.Sprintf("%s <%s>", s.name, s.addr),
				Subject:   fmt.Sprintf("%s #%d", s.subject, j+1),
				Date:      now.AddDate(0, 0, -(id % 90)),
				Size:      2048 + id*512,
				Folder:    s.folder,
				IsRead:    (id % 3) != 0,
			})
			id++
		}
	}
	return emails
}
