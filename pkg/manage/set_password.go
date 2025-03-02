package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OpenSlides/openslides-manage-service/proto"
	"github.com/spf13/cobra"
)

const (
	authHashPath      = "/internal/auth/hash"
	datastorWritePath = "/internal/datastore/writer/write"
)

const setPasswordHelp = `Sets the password of an user

This command sets the password of a user by a given user id.
`

// CmdSetPassword initializes the set-password command.
func CmdSetPassword(cfg *ClientConfig) *cobra.Command {
	var userID int64
	var password string

	cmd := &cobra.Command{
		Use:   "set-password",
		Short: "Sets an user password.",
		Long:  setPasswordHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()

			service := Dial(ctx, cfg.Address)

			req := &proto.SetPasswordRequest{
				UserID:   userID,
				Password: password,
			}

			if _, err := service.SetPassword(ctx, req); err != nil {
				return fmt.Errorf("reset password: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().Int64VarP(&userID, "user_id", "u", 1, "ID of the user account")
	cmd.Flags().StringVarP(&password, "password", "p", "admin", "New password for the user")

	return cmd
}

// SetPassword sets hashes and sets the password
func (s *Server) SetPassword(ctx context.Context, in *proto.SetPasswordRequest) (*proto.SetPasswordResponse, error) {
	hash, err := hashPassword(ctx, s.config, in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	if err := setPassword(ctx, s.config, int(in.UserID), hash); err != nil {
		return nil, fmt.Errorf("set password: %w", err)
	}
	return new(proto.SetPasswordResponse), nil
}

// hashPassword returns the hashed form of password as a JSON.
func hashPassword(ctx context.Context, cfg *ServerConfig, password string) (string, error) {
	reqBody := fmt.Sprintf(`{"toHash": "%s"}`, password)
	reqURL := cfg.AuthURL()
	reqURL.Path = authHashPath
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL.String(), strings.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("creating request to auth service: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = []byte("[can not read body]")
		}
		return "", fmt.Errorf("auth service returned %s: %s", resp.Status, body)
	}

	var respBody struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return respBody.Hash, nil
}

func setPassword(ctx context.Context, cfg *ServerConfig, userID int, hash string) error {
	reqBody := fmt.Sprintf(`{"user_id":0,"information":{},"locked_fields":{},"events":[{"type":"update","fqid":"user/%d","fields":{"password":"%s"}}]}`, userID, hash)
	reqURL := cfg.DatastoreWriterURL()
	reqURL.Path = datastorWritePath
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL.String(), strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = []byte("[can not read body]")
		}
		return fmt.Errorf("datastore writer service returned %s: %s", resp.Status, body)
	}

	return nil
}
