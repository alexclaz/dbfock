package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestSignedRequestUsesS3PathAndPayloadHash(t *testing.T) {
	contents := []byte("-- DBfock workspace backup\n")
	request, err := (&Service{}).signedRequest(context.Background(), "PUT", models.BackupSetting{Endpoint: "https://s3.example.test/base", Bucket: "workspace", Region: "us-east-1"}, "access", "secret", bytes.NewReader(contents), "application/sql")
	if err != nil {
		t.Fatalf("sign request: %v", err)
	}
	if request.URL.Path != "/base/workspace/dbfock/backup.sql" {
		t.Fatalf("path = %q", request.URL.Path)
	}
	digest := sha256.Sum256(contents)
	if request.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected payload hash: %q", request.Header.Get("X-Amz-Content-Sha256"))
	}
	authorization := request.Header.Get("Authorization")
	if !strings.Contains(authorization, "Credential=access/") || !strings.Contains(authorization, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("missing AWS v4 authorization: %q", authorization)
	}
}

func TestParseEndpointRejectsInvalidURL(t *testing.T) {
	if _, err := parseEndpoint("s3.example.test"); err == nil {
		t.Fatal("endpoint without a scheme was accepted")
	}
	if _, err := parseEndpoint("ftp://s3.example.test"); err == nil {
		t.Fatal("non-HTTP endpoint was accepted")
	}
}
