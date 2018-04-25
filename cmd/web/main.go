package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/king-jam/slacko-botto/backend"
	"github.com/king-jam/slacko-botto/queue"
	"github.com/nlopes/slack"
)

var defaultDeleteDelay = 2 * time.Minute

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	dbString := os.Getenv("DATABASE_URL")
	if dbString == "" {
		log.Fatal("$DATABASE_URL must be set")
	}
	dbURL, err := url.Parse(dbString)
	if err != nil {
		log.Fatal("Invalid Database URL format")
	}

	db, err := backend.InitDatabase(dbURL)
	if err != nil {
		log.Fatal("Unable to initialize the Database")
	}
	defer db.Close()

	qc, err := queue.NewQueue(dbURL)
	if err != nil {
		log.Fatal("Unable to initialize the Database")
	}
	defer qc.Close()

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

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "add_to_slack.html", nil)
	})

	router.GET("/auth/redirect", func(c *gin.Context) {
		code := c.Query("code")
		response, err := slack.GetOAuthResponse(clientID, clientSecret, code, redirectURI, false)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		t, err := db.GetTokenDataByUserID(response.UserID)
		if err != nil {
			if err == backend.ErrRecordNotFound {
				err = db.CreateTokenData(&backend.TokenData{
					OAuthResponse: *response,
				})
				if err != nil {
					c.Status(http.StatusInternalServerError)
				}
			} else {
				c.Status(http.StatusInternalServerError)
			}
		} else {
			updated := backend.TokenData{
				OAuthResponse: *response,
			}
			updated.ID = t.ID
			if err := db.UpdateTokenData(&updated); err != nil {
				c.Status(http.StatusInternalServerError)
			}
		}
		c.Redirect(303, "https://"+response.TeamName+".slack.com")
	})

	router.POST("/slashcommand/dr", func(c *gin.Context) {
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		slashCommand.ValidateToken(verificationToken)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		t, err := db.GetTokenDataByUserID(slashCommand.UserID)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		text, delayTime, err := parseText(slashCommand.Text)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		api := slack.New(t.AccessToken)
		params := slack.NewPostMessageParameters()
		params.AsUser = true
		params.Username = slashCommand.UserName
		channel, ts, err := api.PostMessage(slashCommand.ChannelID, text,
			params)
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
		deleteTime := time.Now().Add(delayTime)
		if err := qc.QueueDelayedDelete(t.AccessToken, channel, ts, deleteTime); err != nil {
			c.Status(http.StatusInternalServerError)
		}
		c.Status(http.StatusOK)
	})

	router.POST("/slashcommand/clean", func(c *gin.Context) {
		// slashCommand, err := slack.SlashCommandParse(c.Request)
		// if err != nil {
		// 	c.Status(http.StatusInternalServerError)
		// }
		// slashCommand.ValidateToken(verificationToken)
		// if err != nil {
		// 	c.Status(http.StatusInternalServerError)
		// }
		// t, err := db.GetTokenDataByUserID(slashCommand.UserID)
		// if err != nil {
		// 	c.Status(http.StatusInternalServerError)
		// }
		// handleIt(t.AccessToken, slashCommand)
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

func parseText(rawText string) (string, time.Duration, error) {
	text := strings.Split(rawText, " ")
	if len(text) != 2 {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	minutes, err := strconv.Atoi(text[1])
	if err != nil {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	if len(text) != 2 {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	if minutes < 1 || minutes > 15 {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	delay := time.Minute * time.Duration(minutes)
	return text[0], delay, nil
}
