package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	log "github.com/sirupsen/logrus"

	"github.com/appsbyram/seafoodtruck-slack/pkg"
	"github.com/appsbyram/seafoodtruck-slack/version"

	"go.uber.org/zap"
)

const (
	contentTypeHeader         = "Content-Type"
	contentTypeFormURLEncoded = "application/x-www-form-urlencoded"
	searchURL                 = "https://www.seattlefoodtruck.com/search/%s"
	s3BucketURL               = "https://s3-us-west-2.amazonaws.com/seattlefoodtruck-uploads-prod/%s"
	locationScheduleURL       = "https://www.seattlefoodtruck.com/schedule/%s"
	truckURL                  = "https://www.seattlefoodtruck.com/food-trucks/%s"
	helpCmd                   = "help"
	findTrucksCmd             = "find trucks"
	green                     = "#36a64f"
	today                     = "today"
	tomorrow                  = "tomorrow"
)

var (
	addr             = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	wait             time.Duration
	home, configName string
	srv              *http.Server
	token            string
	api              *slack.Client
	proxy            pkg.API
)

func init() {
	token = os.Getenv("TOKEN")
	api = slack.New(token)
	proxy = pkg.API{
		Scheme:     "https",
		Host:       "www.seattlefoodtruck.com",
		BasePath:   "/api",
		HttpClient: http.DefaultClient,
	}
}

func main() {
	log.Info("START***")
	r := mux.NewRouter()
	r.Methods("POST").
		Path("/").
		Name("HomePost").
		HandlerFunc(homeHandler)

	r.Methods("GET").
		Path("/").
		Name("HomeGet").
		HandlerFunc(homeHandler)

	srv = &http.Server{
		Addr:         *addr,
		WriteTimeout: time.Second * 10,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}
	go func() {
		log.Debugf("Server listening on port %s \n", *addr)
		if err := srv.ListenAndServe(); err != nil {
			log.Errorf("Error starting the server %v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	srv.Shutdown(ctx)
	log.Info("END***")
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	var buffer []byte
	method := strings.ToLower(r.Method)

	switch method {
	case "get":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json := fmt.Sprintf(`{
			"Version": "%s",
			"GitCommitDescription": "%s"
		}`, version.Version, version.GitCommitDescription)

		buffer = []byte(json)
		w.Write(buffer)
		break
	case "post":
		defer r.Body.Close()
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errorf("Error reading payload posted in http request %v", err)
			http.Error(w, "Error reading payload from request", http.StatusBadRequest)
		}

		event, err := slackevents.ParseEvent(json.RawMessage(payload), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Errorf("Error parsing to slack event from payload %v", err)
			http.Error(w, "Error parsing event", http.StatusInternalServerError)
		}
		switch event.Type {
		case slackevents.URLVerification:
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal(payload, &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
			w.Header().Set(contentTypeHeader, contentTypeFormURLEncoded)
			w.Write([]byte(r.Challenge))
			break
		case slackevents.CallbackEvent:
			log.Info("Received event")
			innerEvent := event.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				respond(ev)
			}
			break
		}
		break
	}
}

func formatDate(t time.Time) string {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	t = t.In(loc)
	return t.Format(time.RFC822)
}

func respond(event *slackevents.AppMentionEvent) {
	var loc, at string
	var err error

	log.Infof("Channel: %s", event.Channel)
	text := event.Text
	i := strings.Index(text, ">")

	text = text[i+1 : len(text)]
	log.Infof("Text %s", text)
	if strings.Contains(text, findTrucksCmd) {
		if text, loc, at, err = parseTokensFromMsg(text); err != nil {
			log.Errorf("Error parsing message: %v", zap.Any("Error", err))
		}
	}
	text = strings.TrimSpace(text)
	switch text {
	case helpCmd:
		showHelp(event.Channel)
		break
	case findTrucksCmd:
		findTrucks(event.Channel, loc, at)
		break
	default:
		api.PostMessage(event.Channel, slack.MsgOptionText("Sorry I cannot help you with this, please try help to see things you can ask me",
			false))
	}
}

func findTrucks(channel, loc, at string) {
	var locs = []string{loc}
	var hood string
	var l *pkg.Location
	var n *pkg.Neighborhood
	var err error

	if l, err = proxy.GetLocation(loc); err != nil {
		log.Errorf("Error getting location: %v", err)
		api.PostMessage(channel,
			slack.MsgOptionText("Sorry having issues processing your request. Please try again...", false))
		return
	}
	log.Infof("Neighborhood ID %v for location: %s", l.Neighborhood.ID, loc)
	if n, err = proxy.GetNeighborhood(l.Neighborhood.ID); err != nil {
		log.Errorf("Error getting neighborhood for identifier %v %v", l.Neighborhood.ID, err)
		api.PostMessage(channel,
			slack.MsgOptionText("Sorry having issues processing your request, please try again...", false))
		return
	}
	hood = n.ID
	log.Infof("Got neighborhood id %s. Now finding events booked at location", n.ID)
	if len(loc) > 0 {
		locations, err := proxy.FindTrucks(hood, locs, at)
		if err != nil {
			log.Errorf("Error finding trucks: %v", err)
		}
		log.Infof("Locations : %v", len(locations))
		for _, l := range locations {
			log.Infof("Events %v booked at location %s", len(l.Events), l.Name)
			for _, e := range l.Events {
				st, _ := time.Parse(time.RFC3339, e.StartTime)
				et, _ := time.Parse(time.RFC3339, e.EndTime)
				_, m, d := st.Date()

				title := fmt.Sprintf("*%s* \t %v %v %v - %v \n",
					l.Name, m, d, st.Format(time.Kitchen), et.Format(time.Kitchen))

				var attachments []slack.Attachment
				for _, t := range e.Trucks {
					var fields []slack.AttachmentField
					fields = append(fields, slack.AttachmentField{
						Title: "Food Categories",
						Value: strings.Join(t.FoodCategories, ","),
					})
					attachment := slack.Attachment{
						Color:      green,
						AuthorName: t.Name,
						AuthorLink: fmt.Sprintf(s3BucketURL, t.FeaturedPhoto),
						Title:      t.ID,
						TitleLink:  fmt.Sprintf(truckURL, t.ID),
						Fields:     fields,
					}
					attachments = append(attachments, attachment)
				}
				api.PostMessage(channel, slack.MsgOptionText(title, false), slack.MsgOptionAttachments(attachments...))
				attachments = attachments[:0]
			}
		}
	} else {
		api.PostMessage(channel, slack.MsgOptionText("To find trucks location and neighborhood is required.",
			false))
	}
}

func parseTokensFromMsg(msg string) (string, string, string, error) {
	var cmd, loc, at string
	l := len(msg)
	if l == 0 {
		return "", "", "", errors.New("Message is empty, nothing to do")
	}
	i := strings.Index(msg, " at")
	if i > 0 {
		cmd = msg[0:i]
		cmd = strings.ToLower(cmd)
		cmd = strings.TrimSpace(cmd)
	}
	loc = msg[i+3 : l]
	loc = strings.TrimSpace(loc)
	tokens := strings.Split(loc, " ")
	if len(tokens) == 2 {
		loc = tokens[0]
		at = tokens[1]
	}

	log.Infof("%s, %s, %s", cmd, loc, at)
	return cmd, loc, at, nil
}

func showHelp(channel string) {
	title := "You can ask me"
	commands := fmt.Sprintf("%s \n %s \n", helpCmd,
		findTrucksCmd+" at <location> <today/tomorrow> - to see food trucks at a location")
	attachment := slack.Attachment{
		Color:      green,
		Title:      commands,
		Footer:     "Slack Events API | " + formatDate(time.Now()),
		FooterIcon: "https://platform.slack-edge.com/img/default_application_icon.png",
	}
	api.PostMessage(channel, slack.MsgOptionText(title, false), slack.MsgOptionAttachments(attachment))
}
