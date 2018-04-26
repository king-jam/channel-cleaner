package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/king-jam/slacko-botto/backend"
	"github.com/king-jam/slacko-botto/queue"
	"github.com/nlopes/slack"

	_ "github.com/heroku/x/hmetrics/onload" // heroku metrics
)

var defaultDeleteDelay = 5 * time.Minute
var deployedURL = "https://slacko-botto.herokuapp.com/"

var defaultCleanupOptions = queue.CleanChannelOpts{
	Messages: true,
	Files:    true,
	Bots:     true,
}

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

	qc.InitWorkerPool(2)

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

	// Catch signal so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

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
		if !slashCommand.ValidateToken(verificationToken) {
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
		if !slashCommand.ValidateToken(verificationToken) {
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
		slashCommand, err := slack.SlashCommandParse(c.Request)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if !slashCommand.ValidateToken(verificationToken) {
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
		opts, err := parseCleanChannelOptions(slashCommand.Text)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if err := qc.QueueCleanChannel(t.AccessToken, slashCommand.ChannelID, slashCommand.UserID, opts); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, slack.Msg{
			ResponseType: "ephemeral",
			Text:         "Cleanup Request Scheduled",
		})
	})

	go qc.StartWorkers()

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		// service connections
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen Error: %s\n", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Fatal("Server Shutdown:", err)
		}
	}()

	// Wait for a signal
	sig := <-sigCh
	log.Printf("%s Signal received. Shutting down Application.", sig.String())
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

func parseCleanChannelOptions(rawText string) (queue.CleanChannelOpts, error) {
	if rawText == "" {
		return defaultCleanupOptions, nil
	}
	text := strings.Split(rawText, " ")
	if len(text) == 0 {
		// using defaults
		return defaultCleanupOptions, nil
	}
	if len(text) != 3 {
		return queue.CleanChannelOpts{}, fmt.Errorf("Invalid Request")
	}
	delMsgs, err := strconv.ParseBool(text[0])
	if err != nil {
		return queue.CleanChannelOpts{}, fmt.Errorf("Invalid Request")
	}
	delFiles, err := strconv.ParseBool(text[1])
	if err != nil {
		return queue.CleanChannelOpts{}, fmt.Errorf("Invalid Request")
	}
	delBotMsgs, err := strconv.ParseBool(text[2])
	if err != nil {
		return queue.CleanChannelOpts{}, fmt.Errorf("Invalid Request")
	}
	return queue.CleanChannelOpts{
		Messages: delMsgs,
		Files:    delFiles,
		Bots:     delBotMsgs,
	}, nil
}
