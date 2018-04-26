package queue

import (
	"encoding/json"
	"net/url"
	"time"

	que "github.com/bgentry/que-go"
	"github.com/jackc/pgx"
)

var (
	// CleanChannelJob describes channel cleanup requests
	CleanChannelJob = "CleanChannelRequests"
	// DelayedDeleteJob describes delayed delete requests
	DelayedDeleteJob = "DelayedDeleteRequests"
)

// DelayedDeleteRequest is the struct for doing a delayed delete
type DelayedDeleteRequest struct {
	Token     string `json:"token"`
	Channel   string `json:"channel_id"`
	Timestamp string `json:"ts"`
}

// CleanChannelRequest is the struct for doing a channel cleanup
type CleanChannelRequest struct {
	Token   string           `json:"token"`
	Channel string           `json:"channel_id"`
	UserID  string           `json:"user_id"`
	Options CleanChannelOpts `json:"command_options"`
}

// Queue is a job queue to pass messages between the web thread and workers
type Queue struct {
	qc      *que.Client
	pgxpool *pgx.ConnPool
	wm      *que.WorkMap
	workers *que.WorkerPool
}

// NewQueue initializes and creates a new message passing queue
func NewQueue(dbURL *url.URL) (*Queue, error) {
	pgxpool, qc, err := setupDB(dbURL.String())
	if err != nil {
		return nil, err
	}
	wm := &que.WorkMap{
		DelayedDeleteJob: delayedDelete,
		CleanChannelJob:  cleanchannel,
	}
	return &Queue{
		qc:      qc,
		pgxpool: pgxpool,
		wm:      wm,
	}, nil
}

// Close cleanups up the queue
func (q *Queue) Close() {
	if q.pgxpool != nil {
		q.pgxpool.Close()
	}
	if q.workers != nil {
		q.workers.Shutdown()
	}
}

// QueueCleanChannel enqueues a cleanup channel job
func (q *Queue) QueueCleanChannel(token, channel, userID string, options CleanChannelOpts) error {
	req := CleanChannelRequest{
		Token:   token,
		Channel: channel,
		UserID:  userID,
		Options: options,
	}
	args, err := json.Marshal(req)
	if err != nil {
		return err
	}
	j := que.Job{
		Type: CleanChannelJob,
		Args: args,
	}
	return q.qc.Enqueue(&j)
}

// QueueDelayedDelete enqueues a delayed message delete job
func (q *Queue) QueueDelayedDelete(token, channel, ts string, runAt time.Time) error {
	req := DelayedDeleteRequest{
		Token:     token,
		Channel:   channel,
		Timestamp: ts,
	}
	args, err := json.Marshal(req)
	if err != nil {
		return err
	}
	j := que.Job{
		Type:  DelayedDeleteJob,
		Args:  args,
		RunAt: runAt,
	}
	return q.qc.Enqueue(&j)
}

// InitWorkerPool initializes a worker pool to do work
func (q *Queue) InitWorkerPool(numWorkers int) {
	if q.wm == nil {
		return
	}
	q.workers = que.NewWorkerPool(q.qc, *q.wm, numWorkers)
}

// StartWorkers starts up the worker pool
func (q *Queue) StartWorkers() {
	if q.workers != nil {
		q.workers.Start()
	}
}

// getPgxPool based on the provided database URL
func getPgxPool(dbURL string) (*pgx.ConnPool, error) {
	pgxcfg, err := pgx.ParseURI(dbURL)
	if err != nil {
		return nil, err
	}

	pgxpool, err := pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig:   pgxcfg,
		AfterConnect: que.PrepareStatements,
	})

	if err != nil {
		return nil, err
	}

	return pgxpool, nil
}

// setupDB a *pgx.ConnPool and *que.Client
// This is here so that setup routines can easily be shared between web and
// workers
func setupDB(dbURL string) (*pgx.ConnPool, *que.Client, error) {
	pgxpool, err := getPgxPool(dbURL)
	if err != nil {
		return nil, nil, err
	}

	qc := que.NewClient(pgxpool)

	return pgxpool, qc, err
}
