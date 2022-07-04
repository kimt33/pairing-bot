package main

import (
	"context"
	"fmt"
	"strings"
	"strconv"
)

const helpMessage string = "**How to use Pairing Bot:**\n* `subscribe` to start getting matched with other Pairing Bot users for pair programming\n* `schedule monday wednesday friday` to set your weekly pairing schedule\n  * In this example, I've been set to find pairing partners for you on every Monday, Wednesday, and Friday\n  * You can schedule pairing for any combination of days in the week\n* `streams` to select streams/topics of the match and to select the number of pairings per keyword\n  * For example, `streams any 2 pairing 1 math 1` would schedule per day 2 pairings with anyone, 1 pairing with someone interesting in pair programming, and 1 pairing with someone who'd like to talk about math. Of course, they would need to be available on a given day.\n  * At the moment, there's no strict rules for words as topics here except that they have to be one word. I suggest using the stream name without the spaces!\n* `skip tomorrow` to skip pairing tomorrow\n  * This is valid until matches go out at 04:00 UTC\n* `unskip tomorrow` to undo skipping tomorrow\n* `status` to show your current schedule, skip status, and name\n* `unsubscribe` to stop getting matched entirely\n\nIf you've found a bug, please [submit an issue on github](https://github.com/thwidge/pairing-bot/issues)!"
const subscribeMessage string = "Yay! You're now subscribed to Pairing Bot!\nCurrently, I'm set to find pair programming partners for you on **Mondays**, **Tuesdays**, **Wednesdays**, **Thursdays**, and **Fridays**.\nYou can customize your schedule any time with `schedule` :)"
const unsubscribeMessage string = "You're unsubscribed!\nI won't find pairing partners for you unless you `subscribe`.\n\nBe well :)"
const notSubscribedMessage string = "You're not subscribed to Pairing Bot <3"

var writeErrorMessage = fmt.Sprintf("Something went sideways while writing to the database. You should probably ping %v", owner)
var readErrorMessage = fmt.Sprintf("Something went sideways while reading from the database. You should probably ping %v", owner)

func dispatch(ctx context.Context, pl *PairingLogic, cmd string, cmdArgs []string, userID string, userEmail string, userName string) (string, error) {
	var response string
	var err error

	rec, err := pl.rdb.GetByUserID(ctx, userID, userEmail, userName)
	if err != nil {
		response = readErrorMessage
		return response, err
	}

	isSubscribed := rec.isSubscribed

	// here's the actual actions. command input from
	// the user input has already been sanitized, so we can
	// trust that cmd and cmdArgs only have valid stuff in them
	switch cmd {
	case "schedule":
		if !isSubscribed {
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
		rec.schedule = newSchedule

		if err = pl.rdb.Set(ctx, userID, rec); err != nil {
			response = writeErrorMessage
			break
		}
		response = "Awesome, your new schedule's been set! You can check it with `status`."

	case "streams":
		if !isSubscribed {
			response = notSubscribedMessage
			break
		}
		// convert arguments to map from stream to number of pairings per day in that stream
		var newStreams = map[string]int{}
		for i := 0; i < len(cmdArgs); i += 2{
			// convert string to number
			count, _ := strconv.Atoi(cmdArgs[i + 1])
			// store
			newStreams[cmdArgs[i]] = count
		}
		// put it in the database
		rec.streams = newStreams

		if err = pl.rdb.Set(ctx, userID, rec); err != nil {
			response = writeErrorMessage
			break
		}
		response = "Awesome, your topic's been set! You can check it with `status`."

	case "subscribe":
		if isSubscribed {
			response = "You're already subscribed! Use `schedule` to set your schedule."
			break
		}

		if err = pl.rdb.Set(ctx, userID, rec); err != nil {
			response = writeErrorMessage
			break
		}
		response = subscribeMessage

	case "unsubscribe":
		if !isSubscribed {
			response = notSubscribedMessage
			break
		}

		if err := pl.rdb.Delete(ctx, userID); err != nil {
			response = writeErrorMessage
			break
		}
		response = unsubscribeMessage

	case "skip":
		if !isSubscribed {
			response = notSubscribedMessage
			break
		}

		rec.isSkippingTomorrow = true

		if err := pl.rdb.Set(ctx, userID, rec); err != nil {
			response = writeErrorMessage
			break
		}
		response = `Tomorrow: cancelled. I feel you. **I will not match you** for pairing tomorrow <3`

	case "unskip":
		if !isSubscribed {
			response = notSubscribedMessage
			break
		}
		rec.isSkippingTomorrow = false

		if err := pl.rdb.Set(ctx, userID, rec); err != nil {
			response = writeErrorMessage
			break
		}
		response = "Tomorrow: uncancelled! Heckin *yes*! **I will match you** for pairing tomorrow :)"

	case "status":
		if !isSubscribed {
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
		whoami := rec.name

		// get skip status and prepare to write a sentence with it
		var skipStr string
		if rec.isSkippingTomorrow {
			skipStr = " "
		} else {
			skipStr = " not "
		}

		// make a sorted list of their schedule
		var schedule []string
		for _, day := range daysList {
			// this line is a little wild, sorry. it looks so weird because we
			// have to do type assertion on both interface types
			if rec.schedule[strings.ToLower(day)].(bool) {
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

		// get the streams they'd like to talk in and format string to insert
		// streams are stored as a []string first to make sure "any" comes first and for easy formatting
		var streams []string
		if rec.streams["any"] > 0 {
			streams = append(streams, "any")
		}
		for stream, _ := range rec.streams {
			if stream != "any" {
				streams = append(streams, stream)
			}
		}
		// make string print
		var streamsStr string
		for i, stream := range streams {
			if stream == "any" {
				streamsStr += fmt.Sprintf("%v pairings with any recurser", rec.streams[stream])
			} else {
				if streamsStr != "" {
					streamsStr += ", "
				}
				if i == len(streams) - 1 {
					streamsStr += "and "
				}
				streamsStr += fmt.Sprintf("%v pairings with a recurser from stream %v", rec.streams[stream], stream)
			}
		}

		response = fmt.Sprintf("* You're %v\n* You're scheduled for pairing on **%v**\n We'll try and find you %v \n **You're%vset to skip** pairing tomorrow", whoami, scheduleStr, streamsStr, skipStr)

	case "help":
		response = helpMessage
	default:
		// this won't execute because all input has been sanitized
		// by parseCmd() and all cases are handled explicitly above
	}
	return response, err
}
