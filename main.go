package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nlopes/slack"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	token := os.Getenv("SLACK_TOKEN")
	if token == "" {
		log.Fatal("$SLACK_TOKEN must be set")
	}

	validationToken := os.Getenv("VALIDATION_TOKEN")
	if validationToken == "" {
		log.Fatal("$VALIDATION_TOKEN must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())

	/*
		token=gIkuvaNzQIHg97ATvDxqgjtO
		team_id=T0001
		team_domain=example
		enterprise_id=E0001
		enterprise_name=Globular%20Construct%20Inc
		channel_id=C2147483705
		channel_name=test
		user_id=U2147483697
		user_name=Steve
		command=/weather
		text=94070
		response_url=https://hooks.slack.com/commands/1234/5678
		trigger_id=13345224609.738474920.8088930838d88f008e0
	*/

	router.POST("/", func(c *gin.Context) {
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		slashCommand.ValidateToken(validationToken)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		handleIt(token, slashCommand)
		c.Status(http.StatusOK)
	})

	router.Run(":" + port)
}

func handleIt(token string, slashData slack.SlashCommand) error {
	api := slack.New(token)
	params := slack.NewPostMessageParameters()
	params.AsUser = true
	params.Username = slashData.UserName
	channel, ts, err := api.PostMessage(slashData.ChannelID, "random",
		params)
	if err != nil {
		return err
	}
	_, _, _, err = api.UpdateMessage(channel, ts, slashData.Text)
	return err
}
