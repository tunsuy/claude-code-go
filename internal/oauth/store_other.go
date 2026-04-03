//go:build !darwin

package oauth

// KeychainStore on non-darwin platforms falls back to FileStore.
// This type is declared to satisfy any code that references it by name;
// the actual platform selection is in NewTokenStore().
type KeychainStore struct {
	serviceName string
	accountName string
}

func (k *KeychainStore) Load() (*OAuthTokens, error) {
	fs := &FileStore{path: defaultTokenFilePath()}
	return fs.Load()
}

func (k *KeychainStore) Save(tokens *OAuthTokens) error {
	fs := &FileStore{path: defaultTokenFilePath()}
	return fs.Save(tokens)
}

func (k *KeychainStore) Delete() error {
	fs := &FileStore{path: defaultTokenFilePath()}
	return fs.Delete()
}
