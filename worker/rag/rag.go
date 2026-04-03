package rag

import (
	"context"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/rag"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "rag-index",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		Timeout:      15 * time.Minute,
		WorkerFunc:   WorkerIndex,
	})

	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "rag-query",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		Timeout:      15 * time.Minute,
		WorkerFunc:   WorkerQuery,
	})
}

func WorkerIndex(ctx *job.TaskContext) error {
	logger := ctx.Logger()
	var msg rag.IndexMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	logger.Debugf("RAG: index %s", msg.Doctype)
	return rag.Index(ctx.Instance, logger, msg, makePublishFunc())
}

func WorkerQuery(ctx *job.TaskContext) error {
	logger := ctx.Logger()
	var msg rag.QueryMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	logger.Debugf("RAG: query %v", msg)
	return rag.Query(ctx.Instance, logger, msg)
}

func makePublishFunc() rag.PublishFunc {
	svc := rabbitmq.GetService()
	if _, ok := svc.(*rabbitmq.NoopService); ok {
		return nil
	}
	return func(ctx context.Context, msg rag.BrokerMessage) error {
		return svc.Publish(ctx, rabbitmq.PublishRequest{
			ContextName:  msg.ContextName,
			Exchange:     msg.Exchange,
			RoutingKey:   msg.RoutingKey,
			Headers:      amqp.Table(msg.Headers),
			RawBody:      msg.Body,
			ContentType:  msg.ContentType,
			UnroutableOK: true,
		})
	}
}
