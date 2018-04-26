package queue

import (
	"encoding/json"
	"time"

	que "github.com/bgentry/que-go"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

var rateLimitDelay = 1 * time.Second

// CleanChannelOpts encapsulates all the options for a clean channel command set
type CleanChannelOpts struct {
	Messages bool `json:"delete_messages"`
	Files    bool `json:"delete_files"`
	Bots     bool `json:"delete_bot_messages"`
}

func cleanChannel(j *que.Job) error {
	var ccr CleanChannelRequest
	var more bool
	if err := json.Unmarshal(j.Args, &ccr); err != nil {
		return errors.Wrap(err, "Unable to unmarshal job arguments into CleanChannelRequest: "+string(j.Args))
	}
	api := slack.New(ccr.Token)
	if ccr.Options.Messages || ccr.Options.Bots {
		more = true
		historyParams := &slack.GetConversationHistoryParameters{
			ChannelID: ccr.Channel,
		}
		for more {
			history, err := api.GetConversationHistory(historyParams)
			if err != nil {
				return err
			}
			more = history.HasMore
			if len(history.Messages) == 0 {
				break
			}
			for _, m := range history.Messages {
				// delete messages from the user
				historyParams.Latest = m.Timestamp
				if m.Type == "message" {
					if ccr.Options.Messages {
						if m.User == ccr.UserID {
							_, _, err = api.DeleteMessage(ccr.Channel, m.Timestamp)
							if err != nil {
								return err
							}
							time.Sleep(rateLimitDelay)
						}
					}
					if ccr.Options.Bots {
						if m.SubType == "bot_message" {
							_, _, err = api.DeleteMessage(ccr.Channel, m.Timestamp)
							if err != nil {
								return err
							}
							time.Sleep(rateLimitDelay)
						}
					}
				}
			}
		}
	}
	if ccr.Options.Files {
		more = true
		fileParams := slack.GetFilesParameters{
			User:    ccr.UserID,
			Channel: ccr.Channel,
			Page:    1,
		}
		for more {
			files, paging, err := api.GetFiles(fileParams)
			if err != nil {
				return err
			}
			more = paging.Page < paging.Pages
			fileParams.Page = paging.Page + 1
			for _, f := range files {
				err = api.DeleteFile(f.ID)
				if err != nil {
					return err
				}
				time.Sleep(rateLimitDelay)
			}
		}
	}
	return nil
}
