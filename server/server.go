// Package main implements the gRPC server for the Task Manager application.
// This server handles task submission, status monitoring, and task dispatch.

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	pb "github.com/maciekb2/task-manager/proto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedTaskManagerServer
	rdb *redis.Client
	bus *bus.Client
}

func newServer(rdb *redis.Client, busClient *bus.Client) *server {
	return &server{
		rdb: rdb,
		bus: busClient,
	}
}

func (s *server) SubmitTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	traceParent := traceparentFromContext(ctx)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	idempotencyRedisKey := ""
	idempotencyCreated := false
	taskID := ""
	if idempotencyKey != "" {
		idempotencyRedisKey = "idem:" + idempotencyKey
		taskID = fmt.Sprintf("%d", rand.Int())
		created, err := s.rdb.SetNX(ctx, idempotencyRedisKey, taskID, 24*time.Hour).Result()
		if err != nil {
			return nil, fmt.Errorf("could not store idempotency key: %v", err)
		}
		if !created {
			existingID, err := s.rdb.Get(ctx, idempotencyRedisKey).Result()
			if err != nil {
				return nil, fmt.Errorf("could not read idempotency key: %v", err)
			}
			if span := trace.SpanFromContext(ctx); span != nil {
				span.SetAttributes(
					attribute.String("task.id", existingID),
					attribute.String("task.idempotency_key", idempotencyKey),
					attribute.Bool("task.idempotency_hit", true),
				)
			}
			return &pb.TaskResponse{TaskId: existingID}, nil
		}
		idempotencyCreated = true
	} else {
		taskID = fmt.Sprintf("%d", rand.Int())
	}
	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.String("task.id", taskID),
			attribute.String("task.description", req.TaskDescription),
			attribute.Int("task.priority", int(req.Priority)),
			attribute.Int64("task.number1", int64(req.Number1)),
			attribute.Int64("task.number2", int64(req.Number2)),
			attribute.String("queue.name", bus.SubjectTaskIngest),
			attribute.String("task.idempotency_key", idempotencyKey),
			attribute.Bool("task.idempotency_hit", false),
		)
	}
	newTask := flow.TaskEnvelope{
		TaskID:          taskID,
		TaskDescription: req.TaskDescription,
		Priority:        int32(req.Priority),
		Number1:         req.Number1,
		Number2:         req.Number2,
		TraceParent:     traceParent,
		CreatedAt:       flow.Now(),
	}

	// Store the new task in Redis
	if err := s.rdb.HSet(ctx, "task:"+taskID, map[string]interface{}{
		"id":          newTask.TaskID,
		"description": newTask.TaskDescription,
		"priority":    newTask.Priority,
		"number1":     newTask.Number1,
		"number2":     newTask.Number2,
		"traceparent": newTask.TraceParent,
		"status":      "QUEUED",
	}).Err(); err != nil {
		if idempotencyCreated {
			_ = s.rdb.Del(ctx, idempotencyRedisKey).Err()
		}
		return nil, fmt.Errorf("could not store task: %v", err)
	}

	if _, err := s.bus.PublishJSON(ctx, bus.SubjectTaskIngest, newTask, nil); err != nil {
		if idempotencyCreated {
			_ = s.rdb.Del(ctx, idempotencyRedisKey).Err()
		}
		return nil, fmt.Errorf("could not publish task: %v", err)
	}

	if err := s.enqueueStatus(ctx, flow.StatusUpdate{
		TaskID:      taskID,
		Status:      "QUEUED",
		TraceParent: traceParent,
		Timestamp:   flow.Now(),
		Source:      "gateway",
	}); err != nil {
		log.Printf("Could not enqueue status: %v", err)
	}

	if err := s.enqueueAudit(ctx, flow.AuditEvent{
		TaskID:      taskID,
		Event:       "task.received",
		Detail:      "Task accepted by gateway",
		TraceParent: traceParent,
		Source:      "gateway",
		Timestamp:   flow.Now(),
	}); err != nil {
		log.Printf("Could not enqueue audit event: %v", err)
	}

	return &pb.TaskResponse{TaskId: taskID}, nil
}

func (s *server) StreamTaskStatus(req *pb.StatusRequest, stream pb.TaskManager_StreamTaskStatusServer) error {
	ctx := stream.Context()
	statusKey := flow.StatusChannel(req.TaskId)
	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.String("task.id", req.TaskId),
			attribute.String("channel.name", statusKey),
		)
	}

	if status, err := s.rdb.HGet(ctx, statusKey, "status").Result(); err == nil {
		if err := stream.Send(&pb.StatusResponse{Status: status}); err != nil {
			return err
		}
	}

	pubsub := s.rdb.Subscribe(ctx, statusKey)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			status := msg.Payload
			if err := stream.Send(&pb.StatusResponse{Status: status}); err != nil {
				return err
			}
			if status == "COMPLETED" || status == "FAILED" {
				return nil
			}
		}
	}
}

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "gateway"})
	if err != nil {
		log.Fatalf("nats connect failed: %v", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.TasksStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(serverOpts()...)
	pb.RegisterTaskManagerServer(grpcServer, newServer(rdb, busClient))

	log.Println("Serwer gRPC działa na porcie :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *server) enqueueStatus(ctx context.Context, update flow.StatusUpdate) error {
	_, err := s.bus.PublishJSON(ctx, bus.SubjectEventStatus, update, nil)
	return err
}

func (s *server) enqueueAudit(ctx context.Context, event flow.AuditEvent) error {
	_, err := s.bus.PublishJSON(ctx, bus.SubjectEventAudit, event, nil)
	return err
}

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}

func natsURL() string {
	if addr := os.Getenv("NATS_URL"); addr != "" {
		return addr
	}
	return bus.DefaultURL
}
