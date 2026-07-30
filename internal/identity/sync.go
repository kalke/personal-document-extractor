package identity

import (
	"context"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/store"
)

// Directory adapts store.Users to auth.UserSync.
type Directory struct {
	Users *store.Users
}

func (d Directory) UpsertFromAuth(ctx context.Context, in auth.UserSyncInput) (string, error) {
	u, err := d.Users.UpsertBySubject(ctx, store.UpsertUserInput{
		AuthSubject:   in.Subject,
		Email:         in.Email,
		EmailVerified: in.EmailVerified,
		DisplayName:   in.DisplayName,
	})
	if err != nil {
		return "", err
	}
	if u.Status != "active" {
		return "", auth.ErrUnauthorized
	}
	return u.ID, nil
}

func (d Directory) EnsureActive(ctx context.Context, userID string) error {
	u, err := d.Users.GetByID(ctx, userID)
	if err != nil {
		return auth.ErrUnauthorized
	}
	if u.Status != "active" {
		return auth.ErrUnauthorized
	}
	return nil
}
