package connections

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/encryption"
	"github.com/dbfock/database-manager/backend/internal/models"
	"github.com/dbfock/database-manager/backend/internal/repository"
)

type Input struct {
	Name, Driver, Host, Username, Password, InitialDatabase, Color, Environment string
	Port, TimeoutSeconds                                                        int
	SSLEnabled                                                                  bool
}
type Service struct {
	repo     *repository.Repository
	cipher   *encryption.Service
	registry *database.Registry
}

func New(repo *repository.Repository, cipher *encryption.Service, registry *database.Registry) *Service {
	return &Service{repo, cipher, registry}
}
func validate(in Input) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Username) == "" {
		return fmt.Errorf("name, host and username are required")
	}
	if in.Driver == "" {
		in.Driver = "mysql"
	}
	if in.Driver != "mysql" {
		return fmt.Errorf("only mysql is supported")
	}
	if in.Port < 1 || in.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if in.TimeoutSeconds < 1 || in.TimeoutSeconds > 300 {
		return fmt.Errorf("timeout must be between 1 and 300 seconds")
	}
	if in.Color != "" && !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(in.Color) {
		return fmt.Errorf("color must be a hexadecimal value")
	}
	if in.Environment != "" && in.Environment != "development" && in.Environment != "production" {
		return fmt.Errorf("environment must be development or production")
	}
	if _, _, err := net.SplitHostPort(net.JoinHostPort(in.Host, strconv.Itoa(in.Port))); err != nil {
		return fmt.Errorf("invalid host")
	}
	return nil
}
func connection(in Input) models.Connection {
	color := in.Color
	if color == "" {
		color = "#3B82F6"
	}
	environment := in.Environment
	if environment == "" {
		environment = "development"
	}
	return models.Connection{Name: strings.TrimSpace(in.Name), Driver: in.Driver, Host: strings.TrimSpace(in.Host), Port: in.Port, Username: in.Username, InitialDatabase: in.InitialDatabase, Color: color, Environment: environment, SSLEnabled: in.SSLEnabled, TimeoutSeconds: in.TimeoutSeconds}
}
func (s *Service) Test(ctx context.Context, in Input) error {
	if err := validate(in); err != nil {
		return err
	}
	if in.Password == "" {
		return fmt.Errorf("password is required for a connection test")
	}
	c := connection(in)
	c.PasswordEncrypted = in.Password
	p, err := s.registry.Get(c.Driver)
	if err != nil {
		return err
	}
	return p.TestConnection(ctx, c)
}
func (s *Service) Create(ctx context.Context, in Input) (models.Connection, error) {
	if err := validate(in); err != nil {
		return models.Connection{}, err
	}
	if in.Password == "" {
		return models.Connection{}, fmt.Errorf("password is required")
	}
	encrypted, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return models.Connection{}, err
	}
	c := connection(in)
	c.PasswordEncrypted = encrypted
	return s.repo.CreateConnection(ctx, c)
}

// Import creates a saved connection from an exported configuration. Passwords
// are intentionally optional in export files, so imported connections may need
// a password before they can be used.
func (s *Service) Import(ctx context.Context, in Input) (models.Connection, error) {
	if err := validate(in); err != nil {
		return models.Connection{}, err
	}
	encrypted, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return models.Connection{}, err
	}
	c := connection(in)
	c.PasswordEncrypted = encrypted
	return s.repo.CreateConnection(ctx, c)
}

// ValidateImport checks an exported configuration before any connections are
// written. Unlike Create, an imported password is optional.
func ValidateImport(in Input) error { return validate(in) }
func (s *Service) Update(ctx context.Context, id string, in Input) (models.Connection, error) {
	if err := validate(in); err != nil {
		return models.Connection{}, err
	}
	old, err := s.repo.GetConnection(ctx, repository.LocalUserID, id)
	if err != nil {
		return models.Connection{}, err
	}
	c := connection(in)
	c.ID = old.ID
	c.UserID = old.UserID
	c.PasswordEncrypted = old.PasswordEncrypted
	if in.Password != "" {
		c.PasswordEncrypted, err = s.cipher.Encrypt(in.Password)
		if err != nil {
			return models.Connection{}, err
		}
	}
	return s.repo.UpdateConnection(ctx, c)
}
func (s *Service) GetDecrypted(ctx context.Context, id string) (models.Connection, error) {
	c, err := s.repo.GetConnection(ctx, repository.LocalUserID, id)
	if err != nil {
		return c, err
	}
	plain, err := s.cipher.Decrypt(c.PasswordEncrypted)
	if err != nil {
		return c, err
	}
	c.PasswordEncrypted = plain
	return c, nil
}
func (s *Service) List(ctx context.Context) ([]models.Connection, error) {
	return s.repo.ListConnections(ctx, repository.LocalUserID)
}
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteConnection(ctx, repository.LocalUserID, id)
}
func IsNotFound(err error) bool { return err == sql.ErrNoRows }
