package main

import (
	"context"
	"encoding/json"
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

	"go.uber.org/zap"
)

const (
	contentTypeHeader         = "Content-Type"
	contentTypeFormURLEncoded = "application/x-www-form-urlencoded"
	s3BucketURL               = "https://s3-us-west-2.amazonaws.com/seattlefoodtruck-uploads-prod"
	helpCmd                   = "help"
	showNeighborhoodsCmd      = "show neighborhoods"
	showLocationsCmd          = "show locations"
	showTrucksCmd             = "show trucks"
	green                     = "#00FF00"
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
)

func init() {
	token = os.Getenv("TOKEN")
	logger := logging.FromContext(ctx)
	logger.Infof("Token %s", token)
	api = slack.New(token)
}

func main() {
	logger := logging.FromContext(ctx)
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

	logger := logging.FromContext(ctx)

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
			//api.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
			respond(ev)
		}
		break
	}
}

func attachment(fb, color, title, text, footer string, fields []slack.AttachmentField) slack.Attachment {
	attachment := slack.Attachment{
		Fallback:   fb,
		Color:      color,
		Title:      title,
		Text:       text,
		Footer:     footer,
		FooterIcon: "https://platform.slack-edge.com/img/default_application_icon.png",
		Fields:     fields,
	}
	return attachment
}

func respond(event *slackevents.AppMentionEvent) {
	logger := logging.FromContext(ctx)
	text := event.Text
	i := strings.Index(text, ">")

	text = text[i+1 : len(text)]
	text = strings.TrimSpace(text)
	text = strings.ToLower(text)
	logger.Infof("Text %s", text)

	switch text {
	case helpCmd:
		showHelp(event.Channel)
		break
	}
}

func showHelp(channel string) {
	title := "*You can ask me*"
	commands := fmt.Sprintf("%s \n %s \n %s \n %s", helpCmd, showNeighborhoodsCmd, showLocationsCmd, showTrucksCmd)
	attachment := slack.Attachment{
		Title:      commands,
		Color:      green,
		Footer:     "Slack API",
		FooterIcon: "https://platform.slack-edge.com/img/default_application_icon.png",
	}
	api.PostMessage(channel, slack.MsgOptionText(title, false), slack.MsgOptionAttachments(attachment))
}
