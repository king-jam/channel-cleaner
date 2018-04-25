package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nlopes/slack"
)

var tokenStore map[string]string

func main() {
	tokenStore = make(map[string]string)
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	clientID := os.Getenv("CLIENT_ID")
	if clientID == "" {
		log.Fatal("$CLIENT_ID must be set")
	}

	clientSecret := os.Getenv("CLIENT_SECRET")
	if clientSecret == "" {
		log.Fatal("$CLIENT_SECRET must be set")
	}

	verificationToken := os.Getenv("VERIFICATION_TOKEN")
	if verificationToken == "" {
		log.Fatal("$VERIFICATION_TOKEN must be set")
	}

	redirectURI := os.Getenv("REDIRECT_URI")
	if redirectURI == "" {
		log.Fatal("$REDIRECT_URI must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())

	router.LoadHTMLFiles("static/add_to_slack.html")

	router.GET("/add", func(c *gin.Context) {
		c.HTML(http.StatusOK, "add_to_slack.html", nil)
	})

	router.GET("/auth/redirect", func(c *gin.Context) {
		code := c.Query("code")
		response, err := slack.GetOAuthResponse(clientID, clientSecret, code, redirectURI, false)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		log.Printf("%+v", response)
		tokenStore[response.UserID] = response.AccessToken
		c.Redirect(303, "https://"+response.TeamName+".slack.com")
	})

	router.POST("/slashcommand/sbedit", func(c *gin.Context) {
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		slashCommand.ValidateToken(verificationToken)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		token, ok := tokenStore[slashCommand.UserID]
		if !ok {
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
