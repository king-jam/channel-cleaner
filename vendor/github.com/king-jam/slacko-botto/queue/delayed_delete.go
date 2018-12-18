package queue

import (
	"encoding/json"

	que "github.com/bgentry/que-go"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

func delayedDelete(j *que.Job) error {
	var ddr DelayedDeleteRequest
	if err := json.Unmarshal(j.Args, &ddr); err != nil {
		return errors.Wrap(err, "Unable to unmarshal job arguments into DelayedDeleteRequest: "+string(j.Args))
	}
	api := slack.New(ddr.Token)
	_, _, err := api.DeleteMessage(ddr.Channel, ddr.Timestamp)
	return err
}
