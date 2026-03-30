package backend

import (
	"fmt"
	"time"

	"mail-processor/internal/models"
)

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
