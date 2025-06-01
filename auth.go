package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func getGmailService() (*gmail.Service, error) {
    ctx := context.Background()

    b, err := os.ReadFile("credentials.json")
    if err != nil {
        return nil, fmt.Errorf("unable to read client secret file: %v", err)
    }
    
	config, err := google.ConfigFromJSON(b, gmail.MailGoogleComScope)
	if err != nil {
    return nil, fmt.Errorf("unable to parse client secret file: %v", err)
}

    client, err := getClient(config)
    if err != nil {
        return nil, fmt.Errorf("unable to get client: %v", err)
    }

    srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
    if err != nil {
        return nil, fmt.Errorf("unable to retrieve Gmail client: %v", err)
    }

    return srv, nil
}

func getClient(config *oauth2.Config) (*http.Client, error) {
	tokFile := tokenFilePath()
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok), nil
}

func tokenFilePath() string {
	usr, _ := user.Current()
	return filepath.Join(usr.HomeDir, ".gmail-tui-token.json")
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following URL in your browser then paste the authorization code:\n%v\n", authURL)

	var authCode string
	fmt.Print("Enter code: ")
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		log.Fatalf("Failed to encode token: %v", err)
	}
}