package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nlopes/slack"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("calendar-go-quickstart.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	ctx := context.Background()

	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/calendar-go-quickstart.json
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	srv, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve calendar Client %v", err)
	}

	t := time.Now().Format(time.RFC3339)
	n := time.Now().AddDate(0, 2, 0).Format(time.RFC3339)
	events, err := srv.Events.List("mirantis.com_iqrn5epep3dunclian026s4c6g@group.calendar.google.com").ShowDeleted(false).
		SingleEvents(true).TimeMin(t).TimeMax(n).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("Unable to retrive events information for next 2 months. %v", err)
	}
	CHANNEL_ID := os.Getenv("CHANNEL")
	//TOKEN := os.Getenv("TOKEN")
	//api := slack.New(TOKEN)
	//g, err := api.GetGroupInfo(CHANNEL_ID)
	//if err != nil {
	//	log.Printf("ERROR: get group info: %s", err)
	//}
	//var users map[string]string
	//users = make(map[string]string)
	//for _, i := range g.Members {
	//	u, err := api.GetUserInfo(i)
	//	if err != nil {
	//		log.Printf("ERROR: error during getting user info: %s", err)
	//	}
	// possible bug if RealName is the same for 2 or more people
	//	users[u.RealName] = u.ID
	//}
	fields := make([]slack.AttachmentField, 0)
	fmt.Println("Upcoming events:")
	if len(events.Items) > 0 {
		for _, i := range events.Items {

			var when, end string
			if strings.Contains(i.Summary, "PTO") || strings.Contains(i.Summary, "pto") || strings.Contains(i.Summary, "vacation") {
				// If the DateTime is an empty string the Event is an all-day Event.
				// So only Date is available.

				if i.Start.DateTime != "" {
					when = i.Start.DateTime[0:10]
				} else {
					when = i.Start.Date
				}

				if i.End.DateTime != "" {
					end = i.End.DateTime[0:10]
				} else {
					end = i.End.Date
				}

				fields = append(fields, slack.AttachmentField{
					Title: i.Summary,
					Value: fmt.Sprintf("%s - %s", when, end),
				})

			}
		}
	} else {
		fmt.Printf("No upcoming events found.\n")
	}
	// open Spreadsheet for L2 calendar and add this to the slack message
	attachment := slack.Attachment{
		Pretext: "",
		Text:    fmt.Sprintf("PTO calendar for current 2 months"),
		Fields:  fields,
	}
	params := slack.PostMessageParameters{}
	params.Attachments = []slack.Attachment{attachment}
	var message slack.Msg
	message.Channel = CHANNEL_ID
	message.Text = "<!here>"
	message.PinnedTo = append(message.PinnedTo, CHANNEL_ID)
	message.Attachments = params.Attachments

	body, _ := json.Marshal(message)
	log.Printf("DEBUG: message: %s", body)
	req, err := http.NewRequest("POST", "https://hooks.slack.com/services/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", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	slack_client := &http.Client{}
	resp, err := slack_client.Do(req)
	if err != nil {
		log.Printf("ERROR: send request to web hook %v", err)
	}
	defer resp.Body.Close()
	/*
		_, _, err = api.PostMessage(CHANNEL_ID, "<!here>", params)
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}
	*/
}
