// Copyright 2020 H2O.ai, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keychain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/bcrypt"
)

var (
	idChars                 = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	secretChars             = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	colon                   = []byte(":")
	newline                 = []byte{'\n'}
	errInvalidKeychainEntry = errors.New("invalid entry found in keychain")
)

func generateRandString(chars []byte, n int) (string, error) {
	secret := make([]byte, n)
	rb := make([]byte, n+(n/4))

	k := len(chars)
	umax := 255 - (256 % k)

	i := 0
	for {
		if _, err := rand.Read(rb); err != nil {
			return "", fmt.Errorf("failed generating random bytes: %v", err)
		}
		for _, b := range rb {
			u := int(b)
			if u > umax { // modulo bias
				continue
			}
			secret[i] = chars[u%k]
			i++
			if i == n {
				return string(secret), nil
			}
		}
	}
}

func HashSecret(secret string) ([]byte, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed hashing secret: %v", err)
	}
	return h, nil
}

// Keychain represents a collection of access keys that are allowed to use the API
type Keychain struct {
	Name  string
	keys  map[string][]byte
	cache *lru.Cache
}

func CreateAccessKey() (id, secret string, hash []byte, err error) {
	if id, err = generateRandString(idChars, 20); err != nil {
		return
	}
	if secret, err = generateRandString(secretChars, 40); err != nil {
		return
	}
	hash, err = HashSecret(secret)
	return
}

func (kc *Keychain) Add(id string, hash []byte) {
	kc.keys[id] = hash
}

func (kc *Keychain) verify(id, secret string) bool {
	hash, ok := kc.keys[id]
	if !ok {
		return false
	}

	key := sha512.Sum512([]byte(strings.Join([]string{id, secret}, "\x00")))

	if result, hit := kc.cache.Get(key); hit {
		return result.(bool)
	}

	ok = bcrypt.CompareHashAndPassword(hash, []byte(secret)) == nil
	kc.cache.Add(key, ok)

	return ok
}

func (kc *Keychain) Remove(id string) bool {
	if _, ok := kc.keys[id]; ok {
		delete(kc.keys, id)
		return true
	}
	return false
}

func (kc *Keychain) IDs() []string {
	ids := make([]string, len(kc.keys))
	i := 0
	for id := range kc.keys {
		ids[i] = id
		i++
	}
	return ids
}

func (kc *Keychain) Len() int {
	return len(kc.keys)
}

func newLruCache(size int) (*lru.Cache, error) {
	if size < 8 {
		size = 8
	}
	cache, err := lru.New(size)
	if err != nil {
		return nil, fmt.Errorf("failed creating keychain LRU cache: %s", err)
	}
	return cache, nil
}

func LoadKeychain(name string) (*Keychain, error) {
	keys := make(map[string][]byte)

	if _, err := os.Stat(name); os.IsNotExist(err) {
		cache, err := newLruCache(128)
		if err != nil {
			return nil, err
		}
		return &Keychain{name, keys, cache}, nil
	}

	file, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed opening %s: %v", name, err)
	}
	defer file.Close()

	all, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed reading %s: %v", name, err)
	}

	for _, line := range bytes.Split(all, newline) {
		if len(line) == 0 {
			continue
		}
		tokens := bytes.SplitN(line, colon, 2)
		if len(tokens) != 2 {
			return nil, errInvalidKeychainEntry
		}
		id, hash := tokens[0], tokens[1]
		if len(id) == 0 || len(hash) == 0 {
			return nil, errInvalidKeychainEntry
		}
		keys[string(id)] = hash
	}

	cache, err := newLruCache(len(keys))
	if err != nil {
		return nil, err
	}

	return &Keychain{name, keys, cache}, nil
}

func (kc *Keychain) Save() error {
	var sb bytes.Buffer
	for id, hash := range kc.keys {
		sb.WriteString(id)
		sb.Write(colon)
		sb.Write(hash)
		sb.Write(newline)
	}

	if err := os.WriteFile(kc.Name, sb.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed writing %s: %v", kc.Name, err)
	}

	return nil
}

func (kc *Keychain) Allow(r *http.Request) bool {
	id, secret, ok := r.BasicAuth()
	return ok && kc.verify(id, secret)
}

func (kc *Keychain) Guard(w http.ResponseWriter, r *http.Request) bool {
	if !kc.Allow(r) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return false
	}
	return true
}
