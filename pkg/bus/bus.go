package bus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const DefaultURL = "nats://nats:4222"

var DefaultRetryPolicy = RetryPolicy{
	MaxRetries:     5,
	InitialBackoff: 500 * time.Millisecond,
	MaxBackoff:     30 * time.Second,
	Multiplier:     2,
}

type Config struct {
	URL           string
	Name          string
	Timeout       time.Duration
	MaxReconnects int
	ReconnectWait time.Duration
	User          string
	Password      string
	Token         string
}

type Client struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

type RetryPolicy struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

type ConsumeOptions struct {
	Batch          int
	MaxWait        time.Duration
	Retry          *RetryPolicy
	DLQSubject     string
	DisableAutoAck bool
}

type Handler func(context.Context, *nats.Msg) error

type DLQMessage struct {
	Subject      string              `json:"subject"`
	Reason       string              `json:"reason"`
	Payload      string              `json:"payload"`
	Headers      map[string][]string `json:"headers,omitempty"`
	Stream       string              `json:"stream,omitempty"`
	Consumer     string              `json:"consumer,omitempty"`
	Sequence     uint64              `json:"sequence,omitempty"`
	Timestamp    string              `json:"timestamp,omitempty"`
	NumDelivered uint64              `json:"num_delivered,omitempty"`
	ReceivedAt   string              `json:"received_at"`
}

func Connect(cfg Config, opts ...nats.Option) (*Client, error) {
	url := cfg.URL
	if url == "" {
		url = DefaultURL
	}

	options := make([]nats.Option, 0, 6+len(opts))
	if cfg.Name != "" {
		options = append(options, nats.Name(cfg.Name))
	}
	if cfg.Timeout > 0 {
		options = append(options, nats.Timeout(cfg.Timeout))
	}
	if cfg.MaxReconnects != 0 {
		options = append(options, nats.MaxReconnects(cfg.MaxReconnects))
	}
	if cfg.ReconnectWait > 0 {
		options = append(options, nats.ReconnectWait(cfg.ReconnectWait))
	}
	if cfg.User != "" || cfg.Password != "" {
		options = append(options, nats.UserInfo(cfg.User, cfg.Password))
	}
	if cfg.Token != "" {
		options = append(options, nats.Token(cfg.Token))
	}
	options = append(options, opts...)

	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}

	return &Client{nc: nc, js: js}, nil
}

func (c *Client) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}

func (c *Client) Conn() *nats.Conn {
	if c == nil {
		return nil
	}
	return c.nc
}

func (c *Client) JetStream() nats.JetStreamContext {
	if c == nil {
		return nil
	}
	return c.js
}

func (c *Client) EnsureStream(cfg *nats.StreamConfig) (*nats.StreamInfo, error) {
	if c == nil || c.js == nil {
		return nil, errors.New("jetstream not initialized")
	}
	if cfg == nil {
		return nil, errors.New("stream config is nil")
	}

	info, err := c.js.StreamInfo(cfg.Name)
	if err == nil {
		return c.js.UpdateStream(cfg)
	}
	if errors.Is(err, nats.ErrStreamNotFound) {
		return c.js.AddStream(cfg)
	}
	return info, err
}

func (c *Client) EnsureConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	if c == nil || c.js == nil {
		return nil, errors.New("jetstream not initialized")
	}
	if cfg == nil {
		return nil, errors.New("consumer config is nil")
	}
	if stream == "" {
		return nil, errors.New("stream name is required")
	}

	name := cfg.Durable
	if name == "" {
		name = cfg.Name
	}
	if name == "" {
		return nil, errors.New("consumer durable/name is required")
	}

	info, err := c.js.ConsumerInfo(stream, name)
	if err == nil {
		return c.js.UpdateConsumer(stream, cfg)
	}
	if errors.Is(err, nats.ErrConsumerNotFound) {
		return c.js.AddConsumer(stream, cfg)
	}
	return info, err
}

func (c *Client) Publish(ctx context.Context, subject string, data []byte, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if c == nil || c.js == nil {
		return nil, errors.New("jetstream not initialized")
	}
	msg := &nats.Msg{Subject: subject, Data: data}
	if len(headers) > 0 || ctx != nil {
		msg.Header = cloneHeaders(headers)
		injectTrace(ctx, msg.Header)
	}
	return c.js.PublishMsg(msg, opts...)
}

func (c *Client) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.Publish(ctx, subject, data, headers, opts...)
}

func (c *Client) PullSubscribe(subject, durable string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	if c == nil || c.js == nil {
		return nil, errors.New("jetstream not initialized")
	}
	return c.js.PullSubscribe(subject, durable, opts...)
}

func (c *Client) Consume(ctx context.Context, sub *nats.Subscription, opts ConsumeOptions, handler Handler) error {
	if sub == nil {
		return errors.New("subscription is nil")
	}
	if handler == nil {
		return errors.New("handler is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	batch := opts.Batch
	if batch <= 0 {
		batch = 10
	}
	maxWait := opts.MaxWait
	if maxWait <= 0 {
		maxWait = 5 * time.Second
	}

	retry := DefaultRetryPolicy
	if opts.Retry != nil {
		retry = *opts.Retry
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := sub.Fetch(batch, nats.MaxWait(maxWait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			return err
		}

		for _, msg := range msgs {
			msgCtx := ContextFromHeaders(ctx, msg.Header)
			if err := handler(msgCtx, msg); err != nil {
				handleFailure(c, msg, opts.DLQSubject, retry, err)
				continue
			}
			if !opts.DisableAutoAck {
				_ = msg.Ack()
			}
		}
	}
}

func (c *Client) PublishDLQ(ctx context.Context, subject string, msg *nats.Msg, reason string) error {
	if subject == "" || msg == nil {
		return nil
	}
	payload := BuildDLQMessage(msg, reason)
	_, err := c.PublishJSON(ctx, subject, payload, nil)
	return err
}

func BuildDLQMessage(msg *nats.Msg, reason string) DLQMessage {
	entry := DLQMessage{
		Subject:    msg.Subject,
		Reason:     reason,
		Payload:    base64.StdEncoding.EncodeToString(msg.Data),
		Headers:    cloneHeaders(msg.Header),
		ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	meta, err := msg.Metadata()
	if err == nil && meta != nil {
		entry.Stream = meta.Stream
		entry.Consumer = meta.Consumer
		entry.Sequence = meta.Sequence.Stream
		entry.NumDelivered = meta.NumDelivered
		entry.Timestamp = meta.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return entry
}

func ContextFromHeaders(ctx context.Context, header nats.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(header) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, headerCarrier{header})
}

func injectTrace(ctx context.Context, header nats.Header) {
	if ctx == nil || header == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier{header})
}

func cloneHeaders(header nats.Header) nats.Header {
	if len(header) == 0 {
		return nats.Header{}
	}
	clone := make(nats.Header, len(header))
	for key, values := range header {
		copyValues := make([]string, len(values))
		copy(copyValues, values)
		clone[key] = copyValues
	}
	return clone
}

type headerCarrier struct {
	nats.Header
}

func (c headerCarrier) Get(key string) string {
	return c.Header.Get(key)
}

func (c headerCarrier) Set(key, value string) {
	c.Header.Set(key, value)
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c.Header))
	for k := range c.Header {
		keys = append(keys, k)
	}
	return keys
}

func handleFailure(c *Client, msg *nats.Msg, dlqSubject string, retry RetryPolicy, err error) {
	if msg == nil {
		return
	}

	if retry.MaxRetries <= 0 {
		_ = publishDLQ(c, msg, dlqSubject, err)
		_ = msg.Ack()
		return
	}

	delivered := deliveredCount(msg)
	retryAttempt := delivered - 1
	if retryAttempt >= retry.MaxRetries {
		_ = publishDLQ(c, msg, dlqSubject, err)
		_ = msg.Ack()
		return
	}

	delay := backoffFor(retry, retryAttempt+1)
	_ = nakWithDelay(msg, delay)
}

func publishDLQ(c *Client, msg *nats.Msg, subject string, err error) error {
	if c == nil || subject == "" {
		return nil
	}
	reason := "handler error"
	if err != nil {
		reason = err.Error()
	}
	return c.PublishDLQ(context.Background(), subject, msg, reason)
}

func deliveredCount(msg *nats.Msg) int {
	if msg == nil {
		return 0
	}
	meta, err := msg.Metadata()
	if err != nil || meta == nil || meta.NumDelivered == 0 {
		return 1
	}
	return int(meta.NumDelivered)
}

func DeliveryAttempt(msg *nats.Msg) int {
	return deliveredCount(msg)
}

type nakDelayer interface {
	NakWithDelay(time.Duration) error
}

func nakWithDelay(msg *nats.Msg, delay time.Duration) error {
	if msg == nil {
		return nil
	}
	if delay <= 0 {
		return msg.Nak()
	}
	if withDelay, ok := any(msg).(nakDelayer); ok {
		return withDelay.NakWithDelay(delay)
	}
	return msg.Nak()
}

func backoffFor(policy RetryPolicy, attempt int) time.Duration {
	base := policy.InitialBackoff
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	multiplier := policy.Multiplier
	if multiplier <= 1 {
		multiplier = 2
	}
	if attempt <= 1 {
		return base
	}

	delay := float64(base) * math.Pow(multiplier, float64(attempt-1))
	if policy.MaxBackoff > 0 && delay > float64(policy.MaxBackoff) {
		return policy.MaxBackoff
	}
	return time.Duration(delay)
}

var _ propagation.TextMapCarrier = headerCarrier{}
