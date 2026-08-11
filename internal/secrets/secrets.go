// Package secrets loads a JSON blob from AWS Secrets Manager into the process env.
// Contract mirrors kalke-auth/internal/secrets (FetchMap → ApplyMap; skip if already loaded).
package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const loadedEnv = "KALKE_SECRETS_LOADED"

// MustLoad fetches SECRET_ID when set and merges into the environment.
func MustLoad() error {
	if strings.TrimSpace(os.Getenv(loadedEnv)) != "" {
		return nil
	}
	sid := strings.TrimSpace(os.Getenv("SECRET_ID"))
	if sid == "" {
		return nil
	}
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("aws config: %w", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(sid),
	})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", sid, err)
	}
	raw := ""
	if out.SecretString != nil {
		raw = *out.SecretString
	} else if len(out.SecretBinary) > 0 {
		raw = string(out.SecretBinary)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return fmt.Errorf("secret json: %w", err)
	}
	for k, v := range data {
		if cur, ok := os.LookupEnv(k); ok && cur != "" {
			continue
		}
		switch t := v.(type) {
		case nil:
			continue
		case string:
			_ = os.Setenv(k, t)
		default:
			b, err := json.Marshal(t)
			if err != nil {
				continue
			}
			_ = os.Setenv(k, string(b))
		}
	}
	_ = os.Setenv(loadedEnv, "1")
	return nil
}
