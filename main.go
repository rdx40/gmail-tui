package main

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/api/gmail/v1"
)

func main() {
    // Initialize Gmail service
    srv, err := getGmailService()
    if err != nil {
        log.Fatalf("Failed to initialize Gmail service: %v", err)
    }

    // Retrieve messages in the primary inbox
    msgs, err := srv.Users.Messages.List("me").Q("in:inbox category:primary").MaxResults(10).Do()
    if err != nil {
        log.Fatalf("Unable to retrieve messages: %v", err)
    }

    var emailItems []*gmail.Message
    if msgs != nil && len(msgs.Messages) > 0 {
        emailItems = msgs.Messages
    } else {
        fmt.Println("No messages found in primary inbox.")
    }

    // Get labels
    labels, err := srv.Users.Labels.List("me").Do()
    if err != nil {
        log.Printf("Warning: couldn't fetch labels: %v", err)
        labels = &gmail.ListLabelsResponse{} // Empty labels
    }

    // Initialize the TUI program
    p := tea.NewProgram(initialModel(emailItems, srv, labels.Labels))
    if _, err := p.Run(); err != nil {
        log.Fatalf("Error running TUI: %v", err)
    }
}