package data

import (
  "context"
  "crypto/rand"
  "crypto/sha256"
  "database/sql"
  "time"

  "github.com/kjloveless/greenlight/internal/validator"
)

// Define constants for the token scope. For now we just define the scope
// "activation" but we'll add additional scopes later in the book.
const (
  ScopeActivation     = "activation"
  ScopeAuthentication = "authentication"
)

// Define a Token struct to hold the data for an individual token. This
// includes the plaintext and hashed versions of the token, associated user ID,
// expiry time and scope.
type Token struct {
  Plaintext string    `json:"token"`
  Hash      []byte    `json:"-"`
  UserID    int64     `json:"-"`
  Expiry    time.Time `json:"expiry"`
  Scope     string    `json:"-"`
}

func generateToken(userID int64, ttl time.Duration, scope string) *Token {
  // Create a Token instance. In this, we set the Plaintext field to be a
  // random token generated by rand.Text(), and also set values for the user
  // ID, expiry, and scope of the token. Notice that we add the provided ttl
  // (time-to-live) duration parameter to the current time to get the expiry
  // time?
  token := &Token{
    Plaintext:  rand.Text(),
    UserID:     userID,
    Expiry:     time.Now().Add(ttl),
    Scope:      scope,
  }

  // Generate a SHA-256 hash of the plaintext token string. This will be the
  // value that we store in the `hash` field of our database table. Note that
  // the sha256.Sum256() function returns an *array* of length 32, so to make
  // it easier to work with we convert it to a slice using the [:] operator
  // before storing it.
  hash := sha256.Sum256([]byte(token.Plaintext))
  token.Hash = hash[:]

  return token
}

// Check that the plaintext token has been provided and is exactly 26 bytes
// long.
func ValidateTokenPlaintext(v *validator.Validator, tokenPlaintext string) {
  v.Check(tokenPlaintext != "", "token", "must be provided")
  v.Check(len(tokenPlaintext) == 26, "token", "must be 26 bytes long")
}

// Define the TokenModel type.
type TokenModel struct {
  DB *sql.DB
}

// The New() method is a shortcut which creates a new Token struct and then
// inserts the data in the tokens table.
func (m TokenModel) New(userID int64, ttl time.Duration, scope string) (*Token, error) {
  token := generateToken(userID, ttl, scope)

  err := m.Insert(token)
  return token, err
}

// Insert() adds the data for a specific token to the tokens table.
func (m TokenModel) Insert(token *Token) error {
  query := `
    INSERT INTO tokens (hash, user_id, expiry, scope)
    VALUES ($1, $2, $3, $4)`

  args := []any{ token.Hash, token.UserID, token.Expiry, token.Scope }

  ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
  defer cancel()

  _, err := m.DB.ExecContext(ctx, query, args...)
  return err
}

// DeleteAllForUser() deletes all tokens for a specific user and scope.
func (m TokenModel) DeleteAllForUser(scope string, userID int64) error {
  query := `
    DELETE FROM tokens
    WHERE scope = $1 AND user_id = $2`

  ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
  defer cancel()

  _, err := m.DB.ExecContext(ctx, query, scope, userID)
  return err
}
