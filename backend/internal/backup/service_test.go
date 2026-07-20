package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestSignedRequestUsesS3PathAndPayloadHash(t *testing.T) {
	contents := []byte("-- DBfock workspace backup\n")
	request, err := (&Service{}).signedRequest(context.Background(), "PUT", models.BackupSetting{Endpoint: "https://s3.example.test/base", Bucket: "workspace", Region: "us-east-1"}, "access", "secret", "dbfock/backups/backup-20260720T120000Z.sql", nil, bytes.NewReader(contents), "application/sql")
	if err != nil {
		t.Fatalf("sign request: %v", err)
	}
	if request.URL.Path != "/base/workspace/dbfock/backups/backup-20260720T120000Z.sql" {
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

func TestListSortsTimestampedBackupsAndKeepsLegacyBackup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "2" || r.URL.Query().Get("prefix") != "dbfock/" {
			t.Fatalf("unexpected list request: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>dbfock/backups/backup-older.sql</Key><LastModified>2026-07-19T10:00:00Z</LastModified><Size>10</Size></Contents><Contents><Key>dbfock/backups/backup-newer.sql</Key><LastModified>2026-07-20T10:00:00Z</LastModified><Size>20</Size></Contents><Contents><Key>dbfock/backup.sql</Key><LastModified>2026-07-18T10:00:00Z</LastModified><Size>30</Size></Contents><Contents><Key>dbfock/other.txt</Key><LastModified>2026-07-20T11:00:00Z</LastModified><Size>40</Size></Contents></ListBucketResult>`))
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	backups, err := service.list(context.Background(), models.BackupSetting{Endpoint: server.URL, Bucket: "workspace", Region: "us-east-1"}, "access", "secret")
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("backups = %#v, want 3 items", backups)
	}
	if backups[0].Key != "dbfock/backups/backup-newer.sql" || backups[1].Key != "dbfock/backups/backup-older.sql" || backups[2].Key != legacyObjectKey {
		t.Fatalf("backups not sorted newest first: %#v", backups)
	}
}
