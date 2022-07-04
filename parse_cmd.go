package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"strconv"
)

type parsingErr struct{ msg string }

func (e parsingErr) Error() string {
	return fmt.Sprintf("Error when parsing command: %s", e.msg)
}

func parseCmd(cmdStr string) (string, []string, error) {
	var err error
	var cmdList = []string{
		"subscribe",
		"unsubscribe",
		"help",
		"schedule",
        "streams",
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

    // TODO: how to maintain list of topics? by stream name? free for all text?
    //var streamsList = []string{
    //}

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
	// if there's nothing in the command string array
	case len(cmd) == 0:
		err = errors.New("the user-issued command was blank")
		return "help", nil, err

	// if there's a valid command and if there's no arguments
	case contains(cmdList, cmd[0]) && len(cmd) == 1:
		if cmd[0] == "schedule" || cmd[0] == "streams" || cmd[0] == "skip" || cmd[0] == "unskip" {
			err = &parsingErr{"the user issued a command without args, but it reqired args"}
			return "help", nil, err
		}
		return cmd[0], nil, err

	// if there's a valid command and there's some arguments
	case contains(cmdList, cmd[0]) && len(cmd) > 1:
		switch {
		case cmd[0] == "subscribe" || cmd[0] == "unsubscribe" || cmd[0] == "help" || cmd[0] == "status":
			err = &parsingErr{"the user issued a command with args, but it disallowed args"}
			return "help", nil, err
		case cmd[0] == "skip" && (len(cmd) != 2 || cmd[1] != "tomorrow"):
			err = &parsingErr{"the user issued SKIP with malformed arguments"}
			return "help", nil, err
		case cmd[0] == "unskip" && (len(cmd) != 2 || cmd[1] != "tomorrow"):
			err = &parsingErr{"the user issued UNSKIP with malformed arguments"}
			return "help", nil, err
		case cmd[0] == "schedule":
			for _, v := range cmd[1:] {
				if !contains(daysList, v) {
					err = &parsingErr{"the user issued SCHEDULE with malformed arguments"}
					return "help", nil, err
				}
			}
			fallthrough
		case cmd[0] == "streams":
			// check that number of arguments after "streams" is even
			if len(cmd) == 1 || (len(cmd) - 1) % 2 == 1 {
				err = &parsingErr{"the user issued STREAMS with malformed arguments"}
				return "help", nil, err
			}
			// check that arguments alternate between valid stream and integer
			for i := 1; i < len(cmd); i += 2{
				// FIXME: not sure how to get list of streams from zulip
				//if !contains(streamsList, cmd[i]) {
				//	err = &parsingErr{"the user issued STREAMS with malformed arguments"}
				//	return "help", nil, err
				//}
				// check that next element is integer and convert to appropriate type
				// FIXME: is it possible to make a list of str and ints? list of (str, int)?
				if _, err := strconv.Atoi(cmd[i + 1]); err != nil {
					err = &parsingErr{"the user issued STREAMS with malformed arguments"}
					return "help", nil, err
				}
			}
			fallthrough
		default:
			return cmd[0], cmd[1:], err
		}

	// if there's not a valid command
	default:
		err = &parsingErr{"the user-issued command wasn't valid"}
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
