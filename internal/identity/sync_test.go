package identity_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/identity"
)

func TestDirectoryImplementsUserSync(t *testing.T) {
	var _ auth.UserSync = identity.Directory{}
}
