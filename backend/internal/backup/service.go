// Package backup stores and restores DBfock workspace backups on S3-compatible storage.
package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dbfock/database-manager/backend/internal/encryption"
	"github.com/dbfock/database-manager/backend/internal/models"
	"github.com/dbfock/database-manager/backend/internal/repository"
)

const objectKey = "dbfock/backup.sql"
const maxBackupSize = 50 << 20

type Service struct {
	repo   *repository.Repository
	cipher *encryption.Service
	client *http.Client
}

func New(repo *repository.Repository, cipher *encryption.Service) *Service {
	return &Service{repo: repo, cipher: cipher, client: &http.Client{Timeout: 60 * time.Second}}
}

func (s *Service) Get(ctx context.Context) (models.BackupSetting, error) {
	return s.repo.GetBackupSetting(ctx, repository.LocalUserID)
}

func (s *Service) Save(ctx context.Context, endpoint, bucket, region, accessKey, secret string) (models.BackupSetting, error) {
	endpoint, bucket, region = strings.TrimRight(strings.TrimSpace(endpoint), "/"), strings.TrimSpace(bucket), strings.TrimSpace(region)
	if _, err := parseEndpoint(endpoint); err != nil {
		return models.BackupSetting{}, err
	}
	if bucket == "" || strings.Contains(bucket, "/") {
		return models.BackupSetting{}, fmt.Errorf("a valid bucket is required")
	}
	if region == "" {
		return models.BackupSetting{}, fmt.Errorf("a region is required")
	}
	old, _ := s.repo.GetBackupSetting(ctx, repository.LocalUserID)
	accessEncrypted, secretEncrypted := old.AccessKeyEncrypted, old.SecretEncrypted
	var err error
	if accessKey != "" {
		if accessEncrypted, err = s.cipher.Encrypt(strings.TrimSpace(accessKey)); err != nil {
			return models.BackupSetting{}, err
		}
	}
	if secret != "" {
		if secretEncrypted, err = s.cipher.Encrypt(strings.TrimSpace(secret)); err != nil {
			return models.BackupSetting{}, err
		}
	}
	if accessEncrypted == "" || secretEncrypted == "" {
		return models.BackupSetting{}, fmt.Errorf("access key and secret are required")
	}
	setting := models.BackupSetting{Endpoint: endpoint, Bucket: bucket, Region: region, AccessKeyEncrypted: accessEncrypted, SecretEncrypted: secretEncrypted}
	return setting, s.repo.SaveBackupSetting(ctx, setting)
}

func (s *Service) Create(ctx context.Context) error {
	setting, accessKey, secret, err := s.credentials(ctx)
	if err != nil {
		return err
	}
	script, err := s.repo.DumpSQL(ctx)
	if err != nil {
		return fmt.Errorf("create SQL backup: %w", err)
	}
	if len(script) > maxBackupSize {
		return fmt.Errorf("backup is larger than 50 MB")
	}
	request, err := s.signedRequest(ctx, http.MethodPut, setting, accessKey, secret, bytes.NewReader([]byte(script)), "application/sql; charset=utf-8")
	if err != nil {
		return err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("upload backup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		return s3Error("upload backup", response)
	}
	return nil
}

func (s *Service) Restore(ctx context.Context) error {
	setting, accessKey, secret, err := s.credentials(ctx)
	if err != nil {
		return err
	}
	request, err := s.signedRequest(ctx, http.MethodGet, setting, accessKey, secret, nil, "")
	if err != nil {
		return err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("download backup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		return s3Error("download backup", response)
	}
	script, err := io.ReadAll(io.LimitReader(response.Body, maxBackupSize+1))
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if len(script) > maxBackupSize {
		return fmt.Errorf("backup is larger than 50 MB")
	}
	if err := s.repo.RestoreSQL(ctx, string(script)); err != nil {
		return fmt.Errorf("restore SQL backup: %w", err)
	}
	return nil
}

func (s *Service) credentials(ctx context.Context) (models.BackupSetting, string, string, error) {
	setting, err := s.Get(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return setting, "", "", fmt.Errorf("configure the S3 backup first")
		}
		return setting, "", "", err
	}
	accessKey, err := s.cipher.Decrypt(setting.AccessKeyEncrypted)
	if err != nil {
		return setting, "", "", fmt.Errorf("decrypt S3 access key: %w", err)
	}
	secret, err := s.cipher.Decrypt(setting.SecretEncrypted)
	if err != nil {
		return setting, "", "", fmt.Errorf("decrypt S3 secret: %w", err)
	}
	return setting, accessKey, secret, nil
}

func (s *Service) signedRequest(ctx context.Context, method string, setting models.BackupSetting, accessKey, secret string, body io.Reader, contentType string) (*http.Request, error) {
	endpoint, err := parseEndpoint(setting.Endpoint)
	if err != nil {
		return nil, err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + url.PathEscape(setting.Bucket) + "/" + objectKey
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	emptyPayload := sha256.Sum256(nil)
	payloadHash := hex.EncodeToString(emptyPayload[:])
	if method == http.MethodPut {
		// The request body is already small and has been materialized by Create.
		if reader, ok := body.(*bytes.Reader); ok {
			contents := make([]byte, reader.Len())
			_, _ = reader.Read(contents)
			_, _ = reader.Seek(0, io.SeekStart)
			digest := sha256.Sum256(contents)
			payloadHash = hex.EncodeToString(digest[:])
		}
	}
	now := time.Now().UTC()
	amzDate, dateStamp := now.Format("20060102T150405Z"), now.Format("20060102")
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	request.Header.Set("X-Amz-Date", amzDate)

	canonicalHeaders := "host:" + request.URL.Host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{method, request.URL.EscapedPath(), request.URL.RawQuery, canonicalHeaders, signedHeaders, payloadHash}, "\n")
	credentialScope := dateStamp + "/" + setting.Region + "/s3/aws4_request"
	canonicalDigest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + hex.EncodeToString(canonicalDigest[:])
	derivedKey := signingKey(secret, dateStamp, setting.Region)
	signature := hex.EncodeToString(hmacSHA256(derivedKey, stringToSign))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return request, nil
}

func parseEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, fmt.Errorf("a valid S3 endpoint URL is required")
	}
	return endpoint, nil
}
func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
func signingKey(secret, date, region string) []byte {
	return hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+secret), date), region), "s3"), "aws4_request")
}
func s3Error(action string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
	return fmt.Errorf("%s: %s: %s", action, response.Status, strings.TrimSpace(string(body)))
}
