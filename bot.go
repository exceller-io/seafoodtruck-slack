package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/robfig/cron"

	s "github.com/appsbyram/pkg/http"
	"github.com/appsbyram/pkg/logging"
	"github.com/appsbyram/seafoodtruck-slack/pkg/seattlefoodtruck"
	"github.com/appsbyram/seafoodtruck-slack/version"

	"go.uber.org/zap"
)

const (
	contentTypeHeader         = "Content-Type"
	contentTypeFormURLEncoded = "application/x-www-form-urlencoded"
	s3BucketURL               = "https://s3-us-west-2.amazonaws.com/seattlefoodtruck-uploads-prod/%s"
	locationScheduleURL       = "https://www.seattlefoodtruck.com/schedule/%s"
	truckURL                  = "https://www.seattlefoodtruck.com/food-trucks/%s"
	helpCmd                   = "help"
	findEventsCmd             = "find events"
	green                     = "#36a64f"
	today                     = "today"
	tomorrow                  = "tomorrow"
	blackStar                 = "★"
	whiteStar                 = "☆"
)

var (
	addr             string
	home, configName string
	token            string
	api              *slack.Client
	proxy            seattlefoodtruck.FoodTruckClient
	emojiMapping     = map[string]string{
		"BBQ":             ":cut_of_meat:",
		"Beverage":        ":cup_with_straw:",
		"Burgers":         ":hamburger:",
		"Indian":          ":flag-in:",
		"Vegetarian":      ":green_salad:",
		"Vegan":           ":seedling:",
		"Native American": ":earth_americas:",
		"Asian":           ":earth_asia:",
		"Hawaiian":        ":pineapple:",
		"Seafood":         ":crab:",
		"Sandwiches":      ":sandwich:",
		"Italian":         ":spaghetti:",
		"Pizza":           ":pizza:",
		"Mexican":         ":taco:",
		"Tacos":           ":taco:",
		"Burritos":        ":burrito:",
		"Wraps":           ":burrito:",
		"Sushi":           ":sushi:",
		"Japanese":        ":japan:",
		"Latin American":  ":earth_americas:",
		"Breakfast":       ":fried_egg:",
		"American":        ":flag-us:",
		"Southern":        ":face_with_cowboy_hat:",
		"Caribbean":       ":palm_tree:",
		"Central Asian":   ":earth_asia:",
		"Coffee":          ":coffee:",
		"Dessert":         ":ice_cream:",
		"Ethiopian":       ":flag-et:",
		"European":        ":earth_africa:",
		"French":          ":flag-fr:",
		"Global":          ":globe_with_meridians:",
		"Halal":           "حلال",
		"Hot Dogs":        ":hotdog:",
		"Mediterranean":   ":stuffed_flatbread:",
		"Middle Eastern":  ":stuffed_flatbread:",
	}
	logger    *zap.SugaredLogger
	logLevel  zap.AtomicLevel
	channel   string
	c         *cron.Cron
	locations string
)

func init() {
	token = os.Getenv("TOKEN")
	api = slack.New(token)
	channel = os.Getenv("CHANNEL")
	locations = os.Getenv("LOCATION_IDS")

	flag.StringVar(&addr, "listen-address", ":8080", "The address to listen on for HTTP requests.")
}

func main() {
	logger, logLevel = logging.NewLogger("info")
	ctx := logging.WithLogger(context.TODO(), logger)
	proxy = seattlefoodtruck.NewFoodTruckClient(ctx, "www.seattlefoodtruck.com", "https", "/api")

	routes := s.Routes{
		s.Route{
			"HomeGet",
			"GET",
			"/",
			homeHandler,
		},
		s.Route{
			"HomePost",
			"POST",
			"/",
			homeHandler,
		},
		s.Route{
			"EventsGet",
			"GET",
			"/events",
			eventsHandler,
		},
	}

	//start cron
	startJob()

	srv := s.NewServer(addr, false, "", "", routes)
	srv.Start()
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	day := r.URL.Query().Get("day")

	events, err := proxy.GetEvents(id, day)
	if err != nil {
		http.Error(w, "Error getting events", http.StatusInternalServerError)
	}
	p := s.NewPayload()
	p.WriteResponse(s.ContentTypeJSON, 200, &events, w)
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
			"GitCommitID": "%s"
		}`, version.Version, version.GitCommitID)

		buffer = []byte(json)
		w.Write(buffer)
		break
	case "post":
		defer r.Body.Close()
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Errorw("Error reading payload posted in http request", zap.Error(err))
			http.Error(w, "Error reading payload from request", http.StatusBadRequest)
		}

		event, err := slackevents.ParseEvent(json.RawMessage(payload), slackevents.OptionNoVerifyToken())
		if err != nil {
			logger.Errorw("Error parsing to slack event from payload", zap.Error(err))
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
			logger.Info("Received event")
			innerEvent := event.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				//respond without blocking
				go respond(ev)
			}
			//send http 200k
			w.WriteHeader(http.StatusOK)
			break
		}
		break
	}
}

func formatDateAsPST(t time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		logger.Infow("Error loading location", zap.Error(err))
	} else {
		t = t.In(loc)
	}
	return t.Format(time.RFC822)
}

func respond(event *slackevents.AppMentionEvent) {
	var day string
	var err error

	logger.Infof("Channel: %s", event.Channel)
	text := event.Text
	i := strings.Index(text, ">")

	text = text[i+1 : len(text)]
	logger.Infof("Text %s", text)
	if strings.Contains(text, findEventsCmd) {
		if text, day, err = parseTokensFromMsg(text); err != nil {
			logger.Errorw("Error parsing message: %v", zap.Error(err))
		}
	}
	text = strings.TrimSpace(text)
	switch text {
	case helpCmd:
		showHelp(event.Channel)
		break
	case findEventsCmd:
		postEvents(event.Channel, day)
		break
	default:
		api.PostMessage(event.Channel, slack.MsgOptionText("Sorry I cannot help you with this, please try help to see things you can ask me",
			false))
	}
}

func postEvents(channel, day string) {
	var forLocations []string
	var err error
	var events []seattlefoodtruck.Event
	var loc seattlefoodtruck.Location

	forLocations = strings.Split(locations, ",")

	if len(forLocations) > 0 {
		for i, id := range forLocations {
			loc, err = proxy.GetLocation(id)
			if err != nil {
				api.PostMessage(channel, slack.MsgOptionText("Sorry I'm having trouble getting location details", false))
				return
			}
			events, err = proxy.GetEvents(id, day)
			if err != nil {
				api.PostMessage(channel, slack.MsgOptionText("Sorry I'm having trouble getting events", false))
				return
			}
			if len(events) == 0 {
				logger.Info("No events, skipping")
				continue
			}

			lsURL := fmt.Sprintf(locationScheduleURL, loc.ID)
			ht := fmt.Sprintf("*<%s|%s>*", lsURL, loc.Name)

			htb := slack.NewTextBlockObject("mrkdwn", ht, false, false)
			hsb := slack.NewSectionBlock(htb, nil, nil)
			div := slack.NewDividerBlock()
			msg := slack.NewBlockMessage(
				hsb,
				div,
			)
			for j, e := range events {
				st, _ := time.Parse(time.RFC3339, e.StartTime)
				et, _ := time.Parse(time.RFC3339, e.EndTime)
				_, m, d := st.Date()
				trucks := len(e.Bookings)
				wd := st.Weekday()

				sh := fmt.Sprintf("*%v truck(s)* on %s, %v %v from %v–%v ", trucks, wd.String()[0:3], m, d, st.Format(time.Kitchen), et.Format(time.Kitchen))
				shtb := slack.NewTextBlockObject("mrkdwn", sh, false, false)
				shsb := slack.NewSectionBlock(shtb, nil, nil)
				msg = slack.AddBlockMessage(msg, shsb)

				//loop through each booking and
				for _, b := range e.Bookings {
					var sb strings.Builder

					tURL := fmt.Sprintf(truckURL, b.Truck.ID)
					sb.WriteString(fmt.Sprintf("*<%s|%s>* ", tURL, b.Truck.Name))

					//get truck details
					if truck, err := proxy.GetTruck(b.Truck.ID); err == nil {
						sb.WriteString(fmt.Sprintf("%s (%.1f) %v reviews", getRating(truck.Rating),
							truck.Rating, truck.RatingCount))
					}
					sb.WriteString("\n")
					for _, fc := range b.Truck.FoodCategories {
						emoji := emojiMapping[fc]
						sb.WriteString(fmt.Sprintf("%s %s\n", emoji, fc))
					}
					bhtb := slack.NewTextBlockObject("mrkdwn", sb.String(), false, false)
					//create accessory element
					imgURL := fmt.Sprintf(s3BucketURL, b.Truck.FeaturedPhoto)
					ibe := slack.NewImageBlockElement(imgURL, b.Truck.Name)
					ab := slack.NewAccessory(ibe)
					//create section block
					bhsb := slack.NewSectionBlock(bhtb, nil, ab)

					//add to message
					msg = slack.AddBlockMessage(msg, bhsb)
				}
				if i != len(forLocations) && j == len(events) {
					msg = slack.AddBlockMessage(msg, div)
				}
			}
			api.PostMessage(channel, slack.MsgOptionText("", false), MsgOptionBlocks(msg))
		}
	} else {
		api.PostMessage(channel, slack.MsgOptionText("locations not set",
			false))
	}
}

func parseTokensFromMsg(msg string) (string, string, error) {
	var cmd, day string
	l := len(msg)
	if l == 0 {
		return "", "", errors.New("Message is empty, nothing to do")
	}
	i := strings.Index(msg, " for")
	if i > 0 {
		cmd = msg[0:i]
		cmd = strings.ToLower(cmd)
		cmd = strings.TrimSpace(cmd)
	}
	day = strings.TrimSpace(msg[i+4 : l])
	logger.Infof("Command: %s Day: %s", cmd, day)
	return cmd, day, nil
}

func showHelp(channel string) {
	title := "You can ask me"
	commands := fmt.Sprintf("%s \n %s \n", helpCmd,
		findEventsCmd+" for <today/tomorrow> - to see events booked")
	attachment := slack.Attachment{
		Color:      green,
		Title:      commands,
		Footer:     "Slack Events API | " + formatDateAsPST(time.Now()),
		FooterIcon: "https://platform.slack-edge.com/img/default_application_icon.png",
	}
	_, _, err := api.PostMessage(channel, slack.MsgOptionText(title, false), slack.MsgOptionAttachments(attachment))
	if err != nil {
		logger.Errorw("Error posting message to channel", zap.Error(err))
	}
}

// MsgOptionBlocks applies the blocks from a block message to an existing message.
func MsgOptionBlocks(msg slack.Message) slack.MsgOption {
	return slack.MsgOptionCompose(
		slack.UnsafeMsgOptionEndpoint("", func(v url.Values) {
			blocks, err := json.MarshalIndent(msg.Blocks, "", "    ")
			if err == nil {
				v.Set("blocks", string(blocks))
			}
		}),
		slack.MsgOptionPost(),
	)
}

func getRating(rating float64) string {
	var sb strings.Builder
	r := round(rating)
	for i := 1; i <= 5; i++ {
		if i <= r {
			sb.WriteString(blackStar)
		} else {
			sb.WriteString(whiteStar)
		}
	}
	return sb.String()
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func startJob() {
	if len(locations) > 0 && len(token) > 0 && len(channel) > 0 {
		c = cron.New()
		c.AddFunc("0 0 8 ? * MON-FRI", func() {
			postEvents(channel, today)
		})
		logger.Info("Starting cron job")
		c.Start()
	} else {
		logger.Warn("Cannot start cron job due to missing config values")
	}
}
