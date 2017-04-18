package main

import (
	//"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/nlopes/slack"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/sheets/v4"
)

func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	tok, _ := tokenFromFile(*credential)
	return config.Client(ctx, tok)
}

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

func init() {
	spreadsheetId = flag.String("spreadsheetid", "15WRnM6FkhPaye8sDnZifWf_VpavDBzR4oAYKvLqwn0A", "spreadsheet id for US L1 Team")
	calendarid = flag.String("calendarid", "mirantis.com_iqrn5epep3dunclian026s4c6g@group.calendar.google.com", "Mirantis Operation calendar id")
	token = flag.String("token", "client_secret.json", "client secret token")
	credential = flag.String("crednetial", "calendar-go-quickstart.json", "cache token file")
	verificationToken = flag.String("verification_token", "", "Slack Verification Token")
}

var (
	spreadsheetId                        *string
	calendarid                           *string
	token, credential, verificationToken *string
)

type Vacation struct {
	Start time.Time
	End   time.Time
}
type Attachment struct {
	Text string `json:"text"`
}
type Message struct {
	ResponseType string `json:"response_type,omitempty"`
	slack.Msg
}
type spreadsheets struct {
	s *sheets.Service
}
type ptoCalendar struct {
	s *calendar.Service
}

func NewSpreadsheet(client *http.Client) (s spreadsheets, err error) {
	spreadsheet, err := sheets.New(client)
	if err != nil {
		return s, err
	}
	s.s = spreadsheet
	return s, nil
}
func NewCalendar(client *http.Client) (s ptoCalendar, err error) {
	calendar, err := calendar.New(client)
	if err != nil {
		return s, err
	}
	s.s = calendar
	return s, nil
}
func (s ptoCalendar) getPtoCalendar(calendarid, begin, end string, filter []string) (fields []slack.AttachmentField, err error) {
	events, err := s.s.Events.List(calendarid).ShowDeleted(false).MaxResults(2500).
		SingleEvents(true).TimeMin(begin).TimeMax(end).OrderBy("startTime").Do()
	if err != nil {
		return fields, err
	}
	if len(events.Items) > 0 {
		for _, i := range events.Items {
			var when, end string
			if strings.Contains(i.Summary, "PTO") || strings.Contains(i.Summary, "vacation") {
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
		fields = append(fields, slack.AttachmentField{
			Title: "",
			Value: fmt.Sprintf("No incomming events"),
		})
	}
	return fields, err
}
func parseRequest(r *http.Request) (s map[string][]string, err error) {
	err = r.ParseForm()
	if err != nil {
		return s, err
	}
	return map[string][]string(r.Form), nil
}

func Pto(w http.ResponseWriter, r *http.Request) {
	commandArguments, err := parseRequest(r)
	if err != nil {
		log.Printf("ERROR: request, can't be parsed - %v", err)
	}
	filters := commandArguments["text"]

	var f []string
	for _, filter := range filters {
		f = strings.Split(filter, " ")
		log.Printf("%v\n", f)
	}

	ctx := context.Background()
	b, err := ioutil.ReadFile(*token)
	if err != nil {
		log.Fatalf("Unable to read client secret file %s: %v", *token, err)
	}
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)
	srv, err := NewCalendar(client)
	if err != nil {
		log.Fatalf("Unable to retrieve calendar Client %v", err)
	}
	t := time.Now()
	begin := t.Format(time.RFC3339)
	end := t.AddDate(0, 2, 0).Format(time.RFC3339)
	fields, err := srv.getPtoCalendar(*calendarid, begin, end, f)
	attachment := slack.Attachment{
		Color:  "#b01408",
		Title:  fmt.Sprintf("PTO schedule:"),
		Fields: fields,
	}

	params := slack.PostMessageParameters{}
	params.Attachments = []slack.Attachment{attachment}
	var message Message
	message.ResponseType = "in_channel"
	message.Attachments = params.Attachments
	body, _ := json.Marshal(message)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(body))
	w.WriteHeader(200)
}
func getMonth(pto Vacation) int64 {
	return int64(pto.Start.Month()) + 8
}
func UpdatePto(w http.ResponseWriter, r *http.Request) {
	users := make(map[string][]Vacation)

	commandArguments, err := parseRequest(r)
	if err != nil {
		log.Printf("ERROR: request, can't be parsed - %v", err)
	}
	filters := commandArguments["text"]

	var f []string
	for _, filter := range filters {
		f = strings.Split(filter, " ")
	}

	ctx := context.Background()
	b, err := ioutil.ReadFile(*token)
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope, sheets.SpreadsheetsScope)
	if err != nil {
		w.WriteHeader(503)
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)
	srv, err := NewCalendar(client)
	if err != nil {
		w.WriteHeader(503)
		log.Fatalf("Unable to retrieve calendar Client %v", err)
	}
	t := strconv.Itoa(time.Now().Year())
	begin := fmt.Sprintf("%s-01-01T00:00:00Z", t)
	end := fmt.Sprintf("%s-12-31T23:59:59Z", t)
	fields, err := srv.getPtoCalendar(*calendarid, begin, end, f)
	if err != nil {
		w.WriteHeader(503)
		log.Printf("event queury error: %s", err)
	}
	var v Vacation
	for _, pto := range fields {
		who := strings.TrimSpace(strings.Split(pto.Title, "-")[0])
		dates := strings.Split(pto.Value, " - ")
		v.Start, _ = time.Parse("2006-01-02", dates[0])
		endPto, _ := time.Parse("2006-01-02", dates[1])
		v.End = endPto.AddDate(0, 0, -1)
		users[who] = append(users[who], v)
	}
	spr, err := NewSpreadsheet(client)
	if err != nil {
		log.Printf("unable to open a spreadsheet %s", err)
	}
	readRange := "Sheet1"
	resp, err := spr.s.Spreadsheets.Values.Get(*spreadsheetId, readRange).Do()
	if err != nil {
		w.WriteHeader(401)
		log.Printf("Unable to retrieve data from sheet. %v", err)
	}

	//spreadsheetRequest := make([]*sheets.Request, 0)
	//var ptoSpreadsheet map[int64]string
	if len(resp.Values) > 0 {
		for i, row := range resp.Values {
			// user name
			userName := row[0].(string)
			updateRequest := `
{"requests": [
    {"updateCells": {
        "fields": "*",
        "start": {
          "rowIndex": 2,
          "columnIndex": 11,
          "sheetId": 0
        },
        "rows": [{
            "values": [
              { "userEnteredValue": {
                "stringValue": "02/03-04/03"
               }}]}]}}]}
`
			var rb sheets.BatchUpdateSpreadsheetRequest
			err = json.Unmarshal([]byte(updateRequest), &rb)
			if err != nil {
				log.Printf("%s", err)
			}

			ptoSpreadsheet := make(map[int64]string, 12)
			for _, data := range users[userName] {
				m := getMonth(data)
				ptoSpreadsheet[m] = fmt.Sprintf("%s%s-%s\n", ptoSpreadsheet[m], data.Start.Format("01/02"), data.End.Format("01/02"))
			}
			// think how to send all data using one request
			for month, ptoRange := range ptoSpreadsheet {
				rb.Requests[0].UpdateCells.Start.RowIndex = int64(i)
				rb.Requests[0].UpdateCells.Start.ColumnIndex = month
				rb.Requests[0].UpdateCells.Rows[0].Values[0].UserEnteredValue.StringValue = ptoRange
				_, err = spr.s.Spreadsheets.BatchUpdate(*spreadsheetId, &rb).Context(ctx).Do()
				if err != nil {
					log.Printf("error during update: %v", err)
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	//w.Write([]byte(body))
	w.WriteHeader(200)
}

func main() {
	flag.Parse()
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/pto", Pto)
	router.HandleFunc("/update", UpdatePto)
	// profile router
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	http.ListenAndServe(":8082", router)

}
