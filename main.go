package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/api/gmail/v1"
)

// ...existing code...

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

    // Check if there are any messages
    if msgs == nil || len(msgs.Messages) == 0 {
        fmt.Println("No messages found in primary inbox")
        os.Exit(0)
    }

    // Get labels
    labels, err := srv.Users.Labels.List("me").Do()
    if err != nil {
        log.Printf("Warning: couldn't fetch labels: %v", err)
        labels = &gmail.ListLabelsResponse{} // Empty labels
    }

    // Initialize the TUI program
    p := tea.NewProgram(initialModel(msgs.Messages, srv, labels.Labels))
    if _, err := p.Run(); err != nil {
        log.Fatalf("Error running TUI: %v", err)
    }
}

// ...existing code...