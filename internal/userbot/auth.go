package userbot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// terminalAuth authenticates interactively, reading the verification code
// and 2FA password from standard input.
type terminalAuth struct {
	phone string
}

var _ auth.UserAuthenticator = terminalAuth{}

func (t terminalAuth) Phone(ctx context.Context) (string, error) {
	if t.phone == "" {
		return "", errors.New("phone is not set in config (userbot.phone)")
	}
	return t.phone, nil
}

func (t terminalAuth) Password(ctx context.Context) (string, error) {
	return t.prompt("2FA password (hidden input is not supported)"), nil
}

func (t terminalAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (t terminalAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign up is not supported")
}

func (t terminalAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return t.prompt("verification code"), nil
}

func (t terminalAuth) prompt(label string) string {
	fmt.Printf("Enter %s: ", label)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}

func (c *Client) auth(ctx context.Context) error {
	flow := auth.NewFlow(terminalAuth{phone: c.opts.Phone}, auth.SendCodeOptions{})
	return c.client.Auth().IfNecessary(ctx, flow)
}

// AuthTimeout is how long a single interactive auth attempt may take.
const AuthTimeout = 5 * time.Minute
