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

	_ "github.com/heroku/x/hmetrics/onload" // heroku metrics
)

var defaultDeleteDelay = 5 * time.Minute
var deployedURL = "https://slacko-botto.herokuapp.com/"

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
					return
				}
			} else {
				c.Status(http.StatusInternalServerError)
				return
			}
		} else {
			updated := backend.TokenData{
				OAuthResponse: *response,
			}
			updated.ID = t.ID
			if err := db.UpdateTokenData(&updated); err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
		}
		c.Redirect(303, "https://"+response.TeamName+".slack.com")
	})

	router.POST("/slashcommand/tmp", func(c *gin.Context) {
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		slashCommand.ValidateToken(verificationToken)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		t, err := db.GetTokenDataByUserID(slashCommand.UserID)
		if err != nil {
			if err == backend.ErrRecordNotFound {
				c.JSON(http.StatusOK, userNotFoundMessage())
				return
			}
			c.Status(http.StatusInternalServerError)
			return
		}
		api := slack.New(t.AccessToken)
		params := slack.NewPostMessageParameters()
		params.AsUser = true
		params.Username = slashCommand.UserName
		_, ts, err := api.PostMessage(slashCommand.ChannelID, slashCommand.Text,
			params)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		deleteTime := time.Now().Add(defaultDeleteDelay)
		if err := qc.QueueDelayedDelete(t.AccessToken, slashCommand.ChannelID, ts, deleteTime); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	router.POST("/slashcommand/tmpt", func(c *gin.Context) {
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		slashCommand.ValidateToken(verificationToken)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		t, err := db.GetTokenDataByUserID(slashCommand.UserID)
		if err != nil {
			if err == backend.ErrRecordNotFound {
				c.JSON(http.StatusOK, userNotFoundMessage())
				return
			}
			c.Status(http.StatusInternalServerError)
			return
		}
		text, delayTime, err := parseTextForTimeout(slashCommand.Text)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		api := slack.New(t.AccessToken)
		params := slack.NewPostMessageParameters()
		params.AsUser = true
		params.Username = slashCommand.UserName
		_, ts, err := api.PostMessage(slashCommand.ChannelID, text,
			params)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		deleteTime := time.Now().Add(delayTime)
		if err := qc.QueueDelayedDelete(t.AccessToken, slashCommand.ChannelID, ts, deleteTime); err != nil {
			c.Status(http.StatusInternalServerError)
			return
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

func userNotFoundMessage() slack.Msg {
	return slack.Msg{
		Text:         "Please authorize this app before continuing: " + deployedURL,
		ResponseType: "ephemeral",
	}
}

func errorResponseMessage(errorString string) slack.Msg {
	return slack.Msg{
		Text:         errorString,
		ResponseType: "ephemeral",
	}
}

func parseTextForTimeout(rawText string) (string, time.Duration, error) {
	text := strings.Split(rawText, " ")
	minutes, err := strconv.Atoi(text[len(text)-1])
	if err != nil {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	if minutes > 15 {
		return "", 0, fmt.Errorf("Invalid Request")
	}
	delay := time.Minute * time.Duration(minutes)
	return strings.Join(text[:len(text)-1], " "), delay, nil
}
