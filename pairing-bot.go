package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// this is my real id (it's not really secret)
const marenID int = 215391
const maren string = `@_**Maren Beam (SP2'19)**`
const helpMessage string = "**How to use Pairing Bot:**\n* `subscribe` to start getting matched with other Pairing Bot users for pair programming\n* `schedule monday wednesday friday` to set your weekly pairing schedule\n  * In this example, I've been set to find pairing partners for you on every Monday, Wednesday, and Friday\n  * You can schedule pairing for any combination of days in the week\n* `skip tomorrow` to skip pairing tomorrow\n  * This is valid until matches go out at 6am\n  * If you issue **skip tomorrow** at 4am on Tuesday, you will not be matched for pairing on Tuesday, but you will be matched for pairing on Wednesday (if Wednesday is in your schedule)\n* `unskip tomorrow` to undo skipping tomorrow\n* `status` to show your current schedule, skip status, and name\n* `unsubscribe` to stop getting matched entirely\n  * FWIW, this also removes you from my database"
const subscribeMessage string = "Yay! You're now subscribed to Pairing Bot!\nCurrently, I'm set to find pair programming partners for you on **Mondays**, **Tuesdays**, **Wednesdays**, and **Thursdays**.\nYou can customize your schedule any time with `schedule`.\n\nThanks for signing up :)"
const unsubscribeMessage string = "You're unsubscribed!\nI won't find pairing partners for you unless you `subscribe`.\n\nBe well :)"
const notSubscribedMessage string = "You're not subscribed to Pairing Bot <3"

var writeErrorMessage = fmt.Sprintf("Something went sideways while writing to the database. You should probably ping %v", maren)
var readErrorMessage = fmt.Sprintf("Something went sideways while reading from the database. You should probably ping %v", maren)

// this is my wrong ID, for testing how pairing-bot
// responds to other users
// const marenID int = 215393

// This is a struct that gets only what
// we need from the incoming JSON payload
type incomingJSON struct {
	Data    string `json:"data"`
	Token   string `json:"token"`
	Trigger string `json:"trigger"`
	Message struct {
		SenderID       int    `json:"sender_id"`
		SenderFullName string `json:"sender_full_name"`
	} `json:"message"`
}

// Zulip has to get JSON back from the bot,
// this does that. An empty message field stops
// zulip from throwing an error at the user that
// messaged the bot, but doesn't send a response
type botResponse struct {
	Message string `json:"content"`
}

func sanityCheck(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) (incomingJSON, error) {
	var userReq incomingJSON
	// Look at the incoming webhook and slurp up the JSON
	// Error if the JSON from Zulip istelf is bad
	err := json.NewDecoder(r.Body).Decode(&userReq)
	if err != nil {
		http.NotFound(w, r)
		return userReq, err
	}

	// validate our zulip-bot token
	// this was manually put into the database before deployment
	doc, err := client.Collection("botauth").Doc("token").Get(ctx)
	if err != nil {
		log.Println("Something weird happend trying to read the auth token from the database")
		return userReq, err
	}
	token := doc.Data()
	if userReq.Token != token["value"] {
		http.NotFound(w, r)
		return userReq, errors.New("unauthorized interaction attempt")
	}
	return userReq, err
}

func dispatch(ctx context.Context, client *firestore.Client, cmd string, cmdArgs []string, userID string, userName string) (string, error) {
	var response string
	var err error
	var recurser = map[string]interface{}{
		"id":                 "string",
		"name":               "string",
		"isSkippingTomorrow": false,
		"schedule": map[string]interface{}{
			"monday":    false,
			"tuesday":   false,
			"wednesday": false,
			"thursday":  false,
			"friday":    false,
			"saturday":  false,
			"sunday":    false,
		},
	}

	// get the users "document" (database entry) out of firestore
	// we temporarily keep it in 'doc'
	doc, err := client.Collection("recursers").Doc(userID).Get(ctx)
	// this says "if there's an error, and if that error was not document-not-found"
	if err != nil && grpc.Code(err) != codes.NotFound {
		response = readErrorMessage
		return response, err
	}
	// if there's a db entry, that means they were already subscribed to pairing bot
	// if there's not, they were not subscribed
	isSubscribed := doc.Exists()

	// if the user is in the database, get their current state into this map
	// also assign their zulip name to the name field, just in case it changed
	if isSubscribed {
		recurser = doc.Data()
		recurser["name"] = userName
	}
	// here's the actual actions. command input from
	// the user has already been sanitized, so we can
	// trust that cmd and cmdArgs only have valid stuff in them
	switch cmd {
	case "schedule":
		if isSubscribed == false {
			response = notSubscribedMessage
			break
		}
		// create a new blank schedule
		var newSchedule = map[string]interface{}{
			"monday":    false,
			"tuesday":   false,
			"wednesday": false,
			"thursday":  false,
			"friday":    false,
			"saturday":  false,
			"sunday":    false,
		}
		// populate it with the new days they want to pair on
		for _, day := range cmdArgs {
			newSchedule[day] = true
		}
		// put it in the database
		recurser["schedule"] = newSchedule
		_, err = client.Collection("recursers").Doc(userID).Set(ctx, recurser, firestore.MergeAll)
		if err != nil {
			response = writeErrorMessage
			break
		}
		response = "Awesome, your new schedule's been set! You can check it with `status`."

	case "subscribe":
		if isSubscribed {
			response = "You're already subscribed! Use `schedule` to set your schedule."
			break
		}

		// recurser isn't really a type, because we're using maps
		// and not struct. but we're using it *as* a type,
		// and this is the closest thing to a definition that occurs
		recurser = map[string]interface{}{
			"id":                 userID,
			"name":               userName,
			"isSkippingTomorrow": false,
			"schedule": map[string]interface{}{
				"monday":    true,
				"tuesday":   true,
				"wednesday": true,
				"thursday":  true,
				"friday":    false,
				"saturday":  false,
				"sunday":    false,
			},
		}
		_, err = client.Collection("recursers").Doc(userID).Set(ctx, recurser)
		if err != nil {
			response = writeErrorMessage
			break
		}
		response = subscribeMessage

	case "unsubscribe":
		if isSubscribed == false {
			response = notSubscribedMessage
			break
		}
		_, err = client.Collection("recursers").Doc(userID).Delete(ctx)
		if err != nil {
			response = writeErrorMessage
			break
		}
		response = unsubscribeMessage

	case "skip":
		if isSubscribed == false {
			response = notSubscribedMessage
			break
		}
		recurser["isSkippingTomorrow"] = true
		_, err = client.Collection("recursers").Doc(userID).Set(ctx, recurser, firestore.MergeAll)
		if err != nil {
			response = writeErrorMessage
			break
		}
		response = `Tomorrow: cancelled. I feel you. **I will not match you** for pairing tomorrow <3`

	case "unskip":
		if isSubscribed == false {
			response = notSubscribedMessage
			break
		}
		recurser["isSkippingTomorrow"] = false
		_, err = client.Collection("recursers").Doc(userID).Set(ctx, recurser, firestore.MergeAll)
		if err != nil {
			response = writeErrorMessage
			break
		}
		response = "Tomorrow: uncancelled! Heckin *yes*! **I will match you** for pairing tomorrow :)"

	case "status":
		if isSubscribed == false {
			response = notSubscribedMessage
			break
		}
		// this particular days list is for sorting and printing the
		// schedule correctly, since it's stored in a map in all lowercase
		var daysList = []string{
			"Monday",
			"Tuesday",
			"Wednesday",
			"Thursday",
			"Friday",
			"Saturday",
			"Sunday"}

		// get their current name
		whoami := recurser["name"]

		// get skip status and prepare to write a sentence with it
		var skipStr string
		if recurser["isSkippingTomorrow"].(bool) {
			skipStr = "are"
		} else {
			skipStr = "are not"
		}

		// make a sorted list of their schedule
		var schedule []string
		for _, day := range daysList {
			// this line is a little wild, sorry. it looks so weird because we
			// have to do type assertion on both interface types
			if recurser["schedule"].(map[string]interface{})[strings.ToLower(day)].(bool) {
				schedule = append(schedule, day)
			}
		}
		// make a lil nice-lookin schedule string
		var scheduleStr string
		for i := range schedule[:len(schedule)-1] {
			scheduleStr += schedule[i] + "s, "
		}
		if len(schedule) > 1 {
			scheduleStr += "and " + schedule[len(schedule)-1] + "s"
		} else if len(schedule) == 1 {
			scheduleStr += schedule[0] + "s"
		}

		response = fmt.Sprintf("You are %v.\nYou are scheduled for pairing on %v.\nYou %v set to skip pairing tomorrow.", whoami, scheduleStr, skipStr)

	case "help":
		response = helpMessage
	default:
		// this won't execute because all input has been sanitized
		// by parseCmd() and all cases are handled explicitly above
	}
	return response, err
}

func handle(w http.ResponseWriter, r *http.Request) {
	responder := json.NewEncoder(w)
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "pairing-bot-242820")
	if err != nil {
		log.Panic(err)
	}
	// sanity check the incoming request
	userReq, err := sanityCheck(ctx, client, w, r)
	if err != nil {
		log.Println(err)
		return
	}
	// for testing only
	// this responds uwu and quits if it's not me
	if userReq.Message.SenderID != marenID {
		err = responder.Encode(botResponse{`uwu`})
		if err != nil {
			log.Println(err)
		}
		return
	}
	if userReq.Trigger != "private_message" {
		err = responder.Encode(botResponse{`plz don't @ me i only do pm's <3`})
		if err != nil {
			log.Println(err)
		}
		return
	}
	// you *should* be able to throw any freakin string array at this thing and get back a valid command for dispatch()
	// if there are no commad arguments, cmdArgs will be nil
	cmd, cmdArgs, err := parseCmd(userReq.Data)
	if err != nil {
		log.Println(err)
	}
	// the tofu and potatoes right here y'all
	response, err := dispatch(ctx, client, cmd, cmdArgs, strconv.Itoa(userReq.Message.SenderID), userReq.Message.SenderFullName)
	if err != nil {
		log.Println(err)
	}
	err = responder.Encode(botResponse{response})
	if err != nil {
		log.Println(err)
	}
}

// It's alive! The application starts here.
func main() {
	http.HandleFunc("/", nope)
	http.HandleFunc("/webhooks", handle)
	http.HandleFunc("/cron", cron)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

func parseCmd(cmdStr string) (string, []string, error) {
	var err error
	var cmdList = []string{
		"subscribe",
		"unsubscribe",
		"help",
		"schedule",
		"skip",
		"unskip",
		"status"}

	var daysList = []string{
		"monday",
		"tuesday",
		"wednesday",
		"thursday",
		"friday",
		"saturday",
		"sunday"}

	// convert the string to a slice
	// after this, we have a value "cmd" of type []string
	// where cmd[0] is the command and cmd[1:] are any arguments
	space := regexp.MustCompile(`\s+`)
	cmdStr = space.ReplaceAllString(cmdStr, ` `)
	cmdStr = strings.TrimSpace(cmdStr)
	cmdStr = strings.ToLower(cmdStr)
	cmd := strings.Split(cmdStr, ` `)

	// Big validation logic -- hellooo darkness my old frieeend
	switch {
	// if there's nothing in the command string srray
	case len(cmd) == 0:
		err = errors.New("the user-issued command was blank")
		return "help", nil, err

	// if there's a valid command and if there's no arguments
	case contains(cmdList, cmd[0]) && len(cmd) == 1:
		if cmd[0] == "schedule" || cmd[0] == "skip" || cmd[0] == "unskip" {
			err = errors.New("the user issued a command without args, but it required args")
			return "help", nil, err
		}
		return cmd[0], nil, err

	// if there's a valid command and there's some arguments
	case contains(cmdList, cmd[0]) && len(cmd) > 1:
		switch {
		case cmd[0] == "subscribe" || cmd[0] == "unsubscribe" || cmd[0] == "help" || cmd[0] == "status":
			err = errors.New("the user issued a command with args, but it disallowed args")
			return "help", nil, err
		case cmd[0] == "skip" && len(cmd) != 2 && cmd[1] != "tomorrow":
			err = errors.New("the user issued SKIP with malformed arguments")
			return "help", nil, err
		case cmd[0] == "unskip" && len(cmd) != 2 && cmd[1] != "tomorrow":
			err = errors.New("the user issued UNSKIP with malformed arguments")
			return "help", nil, err
		case cmd[0] == "schedule":
			for _, v := range cmd[1:] {
				if contains(daysList, v) == false {
					err = errors.New("the user issued SCHEDULE with malformed arguments")
					return "help", nil, err
				}
			}
			fallthrough
		default:
			return cmd[0], cmd[1:], err
		}

	// if there's not a valid command
	default:
		err = errors.New("the user-issued command wasn't valid")
		return "help", nil, err
	}
}

func contains(list []string, cmd string) bool {
	for _, v := range list {
		if v == cmd {
			return true
		}
	}
	return false
}

func nope(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func cron(w http.ResponseWriter, r *http.Request) {
	// Check that the request is originating from within app engine
	// even though the firewall should have us covered
	// https://cloud.google.com/appengine/docs/flexible/go/scheduling-jobs-with-cron-yaml#validating_cron_requests
	if r.Header.Get("X-Appengine-Cron") != "true" {
		http.NotFound(w, r)
		return
	}
	// the real thing starts here. setting up database connection
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "pairing-bot-242820")
	if err != nil {
		log.Panic(err)
	}

	today := strings.ToLower(time.Now().Weekday().String())
	matchQuery := client.Collection("recursers").Where("isSkippingTomorrow", "==", false).Where("schedule."+today, "==", true).Documents(ctx)
	doc, err := matchQuery.GetAll()
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(doc)
}
