package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetUserProfileArgs are the arguments for the get_user_profile tool.
type GetUserProfileArgs struct {
	// UserID is the Telegram user ID.
	UserID int64 `json:"user_id"`
}

// GetUserProfileTool returns the tool descriptor for the agent.
func GetUserProfileTool() Tool {
	return Tool{
		Name:        "get_user_profile",
		Description: "Get profile information about a user: name, username and bio. Use it to learn who is writing before answering.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_id": map[string]any{
					"type":        "integer",
					"description": "Telegram user ID.",
				},
			},
			"required": []string{"user_id"},
		},
	}
}

// ExecuteGetUserProfile parses the agent's arguments and returns the profile
// as a JSON object.
func ExecuteGetUserProfile(ctx context.Context, source ChatSource, raw json.RawMessage) (string, error) {
	var args GetUserProfileArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse get_user_profile args: %w", err)
	}

	if args.UserID == 0 {
		return "", fmt.Errorf("get_user_profile: user_id is required")
	}

	p, err := source.Profile(ctx, args.UserID)
	if err != nil {
		return "", fmt.Errorf("get_user_profile: %w", err)
	}

	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("get_user_profile: marshal result: %w", err)
	}
	return string(data), nil
}
