package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// Headless deployments may inject a 32-byte base64 key from their secret manager.
// The key is never written beside the encrypted tokens or into the configuration.
const TokenStoreKeyEnv = "HERALD_TOKEN_STORE_KEY"

var keyringSet = keyring.Set
var keyringGet = keyring.Get

const tokenService = "Herald OAuth"

type tokenPair struct {
	Access  string
	Refresh string
}

func tokenCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv(TokenStoreKeyEnv))
	if err != nil || len(key) != 32 {
		return nil, errors.New("HERALD_TOKEN_STORE_KEY must be a base64 encoded 32-byte key supplied by your secret manager")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func storeTokens(path, identity, access, refresh string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(absolute + "\x00" + identity))
	id := hex.EncodeToString(sum[:])
	data, err := json.Marshal(tokenPair{access, refresh})
	if err != nil {
		return "", err
	}
	if os.Getenv(TokenStoreKeyEnv) == "" {
		if err := keyringSet(tokenService, id, string(data)); err != nil {
			return "", fmt.Errorf("cannot store OAuth tokens in OS credential store: %w; unlock your keychain or configure %s for headless use", err, TokenStoreKeyEnv)
		}
		return "keyring:" + id, nil
	}
	aead, err := tokenCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nonce, nonce, data, []byte(id))
	dir := path + ".tokens"
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(ciphertext); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, id)); err != nil {
		return "", err
	}
	return "encrypted:" + id, nil
}

func readTokens(path, ref string) (tokenPair, error) {
	var pair tokenPair
	kind, id, ok := strings.Cut(ref, ":")
	raw, err := hex.DecodeString(id)
	if !ok || err != nil || len(raw) != 32 {
		return pair, errors.New("invalid OAuth token reference")
	}
	var data []byte
	switch kind {
	case "keyring":
		value, e := keyringGet(tokenService, id)
		err = e
		data = []byte(value)
	case "encrypted":
		var aead cipher.AEAD
		aead, err = tokenCipher()
		if err != nil {
			break
		}
		data, err = os.ReadFile(filepath.Join(path+".tokens", id))
		if err != nil {
			break
		}
		if len(data) < aead.NonceSize() {
			err = errors.New("truncated encrypted token store")
			break
		}
		data, err = aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(id))
	default:
		err = errors.New("unsupported OAuth token store")
	}
	if err != nil {
		return pair, fmt.Errorf("cannot unlock OAuth tokens: %w", err)
	}
	if err = json.Unmarshal(data, &pair); err != nil {
		return pair, errors.New("invalid OAuth token data")
	}
	return pair, nil
}

func (c *Config) protectedCopy(path string) (*Config, error) {
	copy := *c
	copy.Sources = append([]SourceConfig(nil), c.Sources...)
	protect := func(identity string, access, refresh, ref *string) error {
		if *access == "" && *refresh == "" {
			*ref = ""
			return nil
		}
		stored, err := storeTokens(path, identity, *access, *refresh)
		if err != nil {
			return err
		}
		*ref = stored
		*access = ""
		*refresh = ""
		return nil
	}
	if err := protect("legacy-gmail", &copy.Gmail.AccessToken, &copy.Gmail.RefreshToken, &copy.Gmail.TokenRef); err != nil {
		return nil, err
	}
	for i := range copy.Sources {
		s := &copy.Sources[i]
		if err := protect(fmt.Sprintf("source:%s:%d", s.ID, i), &s.Google.AccessToken, &s.Google.RefreshToken, &s.Google.TokenRef); err != nil {
			return nil, err
		}
	}
	return &copy, nil
}

func (c *Config) hydrateTokens(path string) (bool, error) {
	legacy := false
	hydrate := func(access, refresh *string, ref string) error {
		legacy = legacy || *access != "" || *refresh != ""
		if ref == "" {
			return nil
		}
		pair, err := readTokens(path, ref)
		if err != nil {
			return err
		}
		*access = pair.Access
		*refresh = pair.Refresh
		return nil
	}
	if err := hydrate(&c.Gmail.AccessToken, &c.Gmail.RefreshToken, c.Gmail.TokenRef); err != nil {
		return false, err
	}
	for i := range c.Sources {
		s := &c.Sources[i]
		if err := hydrate(&s.Google.AccessToken, &s.Google.RefreshToken, s.Google.TokenRef); err != nil {
			return false, err
		}
	}
	return legacy, nil
}
