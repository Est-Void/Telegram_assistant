package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LaterArgs are the arguments for the later tool: do something at a specific
// time or after a delay — re-invoke the agent or send a chunk of messages.
type LaterArgs struct {
	// InSeconds is the delay in seconds. Use it or At, not both.
	InSeconds *float64 `json:"in_seconds"`
	// At is an absolute time in RFC3339 format. Use it or InSeconds, not both.
	At *string `json:"at"`

	// CallAgent re-invokes the agent at the scheduled time.
	CallAgent *bool `json:"call_agent"`
	// Messages is a chunk of messages to send at the scheduled time.
	Messages []SendMessageArgs `json:"messages"`
}

// LaterTool returns the tool descriptor for the agent.
func LaterTool() Tool {
	messageSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Message text to send.",
			},
			"reply_to_message_id": map[string]any{
				"type":        "integer",
				"description": "ID of the message to reply to. Omit to send a regular message.",
			},
		},
		"required": []string{"text"},
	}

	return Tool{
		Name:        "later",
		Description: "Do something later: after a delay or at a specific time — re-invoke the agent or send a chunk of messages.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"in_seconds": map[string]any{
					"type":        "number",
					"description": "Delay in seconds. Set this or at, not both.",
				},
				"at": map[string]any{
					"type":        "string",
					"description": "Absolute time in RFC3339 format (e.g. 2026-08-06T15:04:05+02:00). Set this or in_seconds, not both.",
				},
				"call_agent": map[string]any{
					"type":        "boolean",
					"description": "Re-invoke the agent at the scheduled time.",
				},
				"messages": map[string]any{
					"type":        "array",
					"description": "Chunk of messages to send at the scheduled time.",
					"items":       messageSchema,
				},
			},
		},
	}
}

// ExecuteLater parses the agent's arguments, resolves the delay and schedules
// the requested action.
func ExecuteLater(ctx context.Context, deps Deps, raw json.RawMessage) error {
	var args LaterArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("parse later args: %w", err)
	}

	delay, err := resolveLaterDelay(args)
	if err != nil {
		return err
	}

	var task func(ctx context.Context)
	hasAction := false

	if args.CallAgent != nil && *args.CallAgent {
		if deps.ReinvokeAgent == nil {
			return fmt.Errorf("later: call_agent is not configured")
		}
		task = func(ctx context.Context) { _ = deps.ReinvokeAgent(ctx) }
		hasAction = true
	}

	if len(args.Messages) > 0 {
		for i := range args.Messages {
			if args.Messages[i].Text == "" {
				return fmt.Errorf("later: messages[%d]: text is required", i)
			}
		}
		task = func(ctx context.Context) {
			for _, m := range args.Messages {
				rawMsg, err := json.Marshal(m)
				if err != nil {
					return
				}
				_ = ExecuteSendMessage(ctx, deps, rawMsg)
			}
		}
		hasAction = true
	}

	if !hasAction {
		return fmt.Errorf("later: nothing to do (set call_agent or messages)")
	}

	return deps.schedule(ctx, delay, task)
}

func resolveLaterDelay(args LaterArgs) (time.Duration, error) {
	switch {
	case args.InSeconds != nil && args.At != nil:
		return 0, fmt.Errorf("later: set only one of in_seconds or at")
	case args.InSeconds != nil:
		if *args.InSeconds <= 0 {
			return 0, fmt.Errorf("later: in_seconds must be positive")
		}
		return time.Duration(*args.InSeconds * float64(time.Second)), nil
	case args.At != nil:
		t, err := time.Parse(time.RFC3339, *args.At)
		if err != nil {
			return 0, fmt.Errorf("later: invalid at time: %w", err)
		}
		delay := time.Until(t)
		if delay <= 0 {
			return 0, fmt.Errorf("later: at time is in the past")
		}
		return delay, nil
	default:
		return 0, fmt.Errorf("later: set one of in_seconds or at")
	}
}
