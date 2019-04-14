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

	"github.com/appsbyram/pkg/logging"
	"github.com/gorilla/mux"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"

	"github.com/appsbyram/seafoodtruck-slack/pkg"

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
	addr             = flag.String("listen-address", ":80", "The address to listen on for HTTP requests.")
	wait             time.Duration
	home, configName string
	srv              *http.Server
	logger           *zap.SugaredLogger
	ctx              = context.Background()
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
	logger = logging.FromContext(ctx)
	logger.Info("START***")
	r := mux.NewRouter()
	r.Methods("POST").
		Path("/").
		Name("Home").
		HandlerFunc(homeHandler)

	srv = &http.Server{
		Addr:         *addr,
		WriteTimeout: time.Second * 10,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}
	go func() {
		logger.Debugf("Server listening on port %s \n", *addr)
		if err := srv.ListenAndServe(); err != nil {
			logger.Errorw("Error starting the server %v", zap.Any("exception", err))
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	srv.Shutdown(ctx)
	logger.Info("END***")
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Errorw("Error reading payload posted in http request", zap.Any("exception", err))
		http.Error(w, "Error reading payload from request", http.StatusBadRequest)
	}

	event, err := slackevents.ParseEvent(json.RawMessage(payload), slackevents.OptionNoVerifyToken())
	if err != nil {
		logger.Errorw("Error parsing to slack event from payload", zap.Any("exception", err))
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
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			respond(ev)
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
	var loc, hood, at string
	var err error

	logger.Infof("Channel: %s", event.Channel)
	text := event.Text
	i := strings.Index(text, ">")

	text = text[i+1 : len(text)]
	logger.Infof("Text %s", text)
	if strings.Contains(text, findTrucksCmd) {
		if text, loc, hood, at, err = parseTokensFromMsg(text); err != nil {
			logger.Errorf("Error parsing message: %v", zap.Any("Error", err))
		}
	}
	text = strings.TrimSpace(text)
	switch text {
	case helpCmd:
		showHelp(event.Channel)
		break
	case findTrucksCmd:
		findTrucks(event.Channel, loc, hood, at)
		break
	default:
		api.PostMessage(event.Channel, slack.MsgOptionText("Sorry I cannot help you with this, please try help to see things you can ask me",
			false))
	}
}

func findTrucks(channel, loc, hood, at string) {
	var locs = []string{loc}
	if len(loc) > 0 && len(hood) > 0 {
		locations, err := proxy.FindTrucks(hood, locs, at)
		if err != nil {
			logger.Infof("Error finding trucks: %v", zap.Any("error", err))
		}
		logger.Infof("Locations : %v", len(locations))
		for _, l := range locations {
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

func parseTokensFromMsg(msg string) (string, string, string, string, error) {
	var cmd, loc, hood, at string
	l := len(msg)
	if l == 0 {
		return "", "", "", "", errors.New("Message is empty, nothing to do")
	}
	i := strings.Index(msg, " at")
	if i > 0 {
		cmd = msg[0:i]
		cmd = strings.ToLower(cmd)
		cmd = strings.TrimSpace(cmd)
	}
	j := strings.Index(msg, " in")
	if j > 0 && i > 0 {
		loc = msg[i+3 : j]
		hood = msg[j+3 : l]
		hood = strings.TrimSpace(hood)
		tokens := strings.Split(hood, " ")
		if len(tokens) == 2 {
			hood = tokens[0]
			at = tokens[1]
		}
	} else {
		loc = msg[i+3 : l]
	}

	logger.Infof("%s, %s, %s, %s", cmd, loc, hood, at)
	return cmd, loc, hood, at, nil
}

func showHelp(channel string) {
	title := "You can ask me"
	commands := fmt.Sprintf("%s \n %s \n", helpCmd,
		findTrucksCmd+" at <location> in <neighborhood> <at> - to see food trucks at a location")
	attachment := slack.Attachment{
		Color:      green,
		Title:      commands,
		Footer:     "Slack Events API | " + formatDate(time.Now()),
		FooterIcon: "https://platform.slack-edge.com/img/default_application_icon.png",
	}
	api.PostMessage(channel, slack.MsgOptionText(title, false), slack.MsgOptionAttachments(attachment))
}
